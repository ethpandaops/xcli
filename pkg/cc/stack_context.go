package cc

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/ethpandaops/xcli/pkg/ai"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/tui"
	"github.com/sirupsen/logrus"
)

const logHistorySize = 10000

// dockerContainerNames maps observability service names to Docker container names.
var dockerContainerNames = map[string]string{
	constants.ServicePrometheus: constants.ContainerPrometheus,
	constants.ServiceGrafana:    constants.ContainerGrafana,
}

// stackContext bundles all per-stack components so each stack operates
// independently with its own SSE hub, health monitor, log streamer, etc.
type stackContext struct {
	name    string
	label   string
	log     logrus.FieldLogger
	backend StackBackend
	api     *apiHandler
	health  *tui.HealthMonitor // nil for stacks without health monitoring
	logs    *tui.LogStreamer
	sseHub  *SSEHub

	logHistory   []tui.LogLine
	logHistoryMu sync.RWMutex
}

// newStackContext creates a fully wired stack context for the given backend.
func newStackContext(
	log logrus.FieldLogger,
	backend StackBackend,
	health *tui.HealthMonitor,
	redis *RedisAdmin,
) *stackContext {
	l := log.WithField("stack", backend.Name())
	logStreamer := tui.NewLogStreamer()
	sseHub := NewSSEHub(l)

	sc := &stackContext{
		name:    backend.Name(),
		label:   backend.Label(),
		log:     l,
		backend: backend,
		health:  health,
		logs:    logStreamer,
		sseHub:  sseHub,
	}

	sc.api = &apiHandler{
		log:               l,
		backend:           backend,
		redis:             redis,
		aiDefaultProvider: ai.DefaultProvider,
		diagnoseSessions:  make(map[string]*diagnoseSession, 8),
		logHistoryFn: func(service string) []string {
			sc.logHistoryMu.RLock()
			defer sc.logHistoryMu.RUnlock()

			lines := make([]string, 0, len(sc.logHistory))
			for _, l := range sc.logHistory {
				if l.Service == service {
					lines = append(lines, l.Raw)
				}
			}

			return lines
		},
		sseHub: sseHub,
	}

	// Detect if the stack was already running (e.g. docker containers from a previous session)
	// and initialise the stack status accordingly. We require a majority of services to be
	// running to avoid false positives from port conflicts (e.g. Lab detecting Xatu's
	// ClickHouse on a shared port).
	services := backend.GetServices(context.Background())
	running := 0

	for _, svc := range services {
		if svc.Status == stackStatusRunning {
			running++
		}
	}

	if len(services) > 0 && running > len(services)/2 {
		sc.api.stack.status = stackStatusRunning

		l.WithField("running_services", running).Info("detected pre-existing running services")
	}

	return sc
}

// Start begins health monitoring and the broadcast loop for this stack.
func (sc *stackContext) Start(ctx context.Context, wg *sync.WaitGroup) {
	if sc.health != nil {
		sc.health.Start()
	}

	wg.Add(1)

	go sc.broadcastLoop(ctx, wg)
}

// Stop shuts down the SSE hub, health monitor, and log streamer.
func (sc *stackContext) Stop() {
	sc.api.closeDiagnoseSessions()
	sc.sseHub.Stop()

	if sc.health != nil {
		sc.health.Stop()
	}

	sc.logs.Stop()
}

// broadcastLoop consumes health/log/status updates and broadcasts via SSE.
func (sc *stackContext) broadcastLoop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	var healthCh <-chan map[string]tui.HealthStatus
	if sc.health != nil {
		healthCh = sc.health.Output()
	}

	logCh := sc.logs.Output()

	statusTicker := time.NewTicker(2 * time.Second)
	defer statusTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case health, ok := <-healthCh:
			if !ok {
				return
			}

			sc.sseHub.Broadcast("health", health)
		case logLine, ok := <-logCh:
			if !ok {
				return
			}

			sc.appendLog(logLine)
			sc.sseHub.Broadcast("log", logLine)
		case <-statusTicker.C:
			services := sc.backend.GetServices(ctx)
			sc.sseHub.Broadcast("services", services)

			stackStatus := sc.api.getStackStatusData()
			sc.sseHub.Broadcast("stack_status", stackStatus)

			sc.startLogStreaming(ctx)
		}
	}
}

// startLogStreaming starts tailing logs for running services and stops
// streaming for services that are no longer running.
func (sc *stackContext) startLogStreaming(ctx context.Context) {
	services := sc.backend.GetServices(ctx)

	running := make(map[string]bool, len(services))
	for _, svc := range services {
		if svc.Status == "running" {
			running[svc.Name] = true
		}
	}

	for _, name := range sc.logs.ActiveServices() {
		if !running[name] {
			sc.logs.StopService(name)
		}
	}

	for _, svc := range services {
		if svc.Status != "running" {
			continue
		}

		src := sc.backend.LogSource(svc.Name)

		switch src.Type {
		case "docker":
			if err := sc.logs.StartDocker(svc.Name, src.Container); err != nil {
				sc.log.WithError(err).WithField(
					"service", svc.Name,
				).Warn("Failed to start Docker log streaming")
			}
		case "file":
			if src.Path != "" {
				if err := sc.logs.Start(svc.Name, src.Path); err != nil {
					sc.log.WithError(err).WithField(
						"service", svc.Name,
					).Warn("Failed to start log streaming")
				}
			}
		}
	}
}

// appendLog adds a log line to the history ring buffer.
func (sc *stackContext) appendLog(line tui.LogLine) {
	sc.logHistoryMu.Lock()
	defer sc.logHistoryMu.Unlock()

	if len(sc.logHistory) >= logHistorySize {
		sc.logHistory = sc.logHistory[1:]
	}

	sc.logHistory = append(sc.logHistory, line)
}

// handleGetLogs returns recent log history so new clients can catch up
// on logs emitted before their SSE connection was established.
func (sc *stackContext) handleGetLogs(w http.ResponseWriter, _ *http.Request) {
	sc.logHistoryMu.RLock()
	logs := make([]tui.LogLine, len(sc.logHistory))
	copy(logs, sc.logHistory)
	sc.logHistoryMu.RUnlock()

	writeJSON(w, http.StatusOK, logs)
}
