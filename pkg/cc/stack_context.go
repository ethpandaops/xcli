package cc

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/git"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/tui"
	"github.com/sirupsen/logrus"
)

const logHistorySize = 1000

// dockerContainerNames maps observability service names to Docker container names.
var dockerContainerNames = map[string]string{
	constants.ServicePrometheus: constants.ContainerPrometheus,
	constants.ServiceGrafana:    constants.ContainerGrafana,
}

// stackContext bundles all per-stack components so each stack operates
// independently with its own SSE hub, health monitor, log streamer, etc.
type stackContext struct {
	name   string
	label  string
	log    logrus.FieldLogger
	api    *apiHandler
	health *tui.HealthMonitor
	logs   *tui.LogStreamer
	sseHub *SSEHub

	logHistory   []tui.LogLine
	logHistoryMu sync.RWMutex
}

// newStackContext creates a fully wired stack context for the given stack.
func newStackContext(
	log logrus.FieldLogger,
	name, label string,
	orch *orchestrator.Orchestrator,
	labCfg *config.LabConfig,
	cfgPath string,
	gitChk *git.Checker,
) *stackContext {
	l := log.WithField("stack", name)
	wrapper := tui.NewOrchestratorWrapper(orch)
	healthMon := tui.NewHealthMonitor(wrapper)
	logStreamer := tui.NewLogStreamer()
	sseHub := NewSSEHub(l)

	api := &apiHandler{
		log:     l,
		wrapper: wrapper,
		health:  healthMon,
		orch:    orch,
		labCfg:  labCfg,
		cfgPath: cfgPath,
		gitChk:  gitChk,
		sseHub:  sseHub,
	}

	return &stackContext{
		name:   name,
		label:  label,
		log:    l,
		api:    api,
		health: healthMon,
		logs:   logStreamer,
		sseHub: sseHub,
	}
}

// Start begins health monitoring and the broadcast loop for this stack.
func (sc *stackContext) Start(ctx context.Context, wg *sync.WaitGroup) {
	sc.health.Start()

	wg.Add(1)

	go sc.broadcastLoop(ctx, wg)
}

// Stop shuts down the SSE hub, health monitor, and log streamer.
func (sc *stackContext) Stop() {
	sc.sseHub.Stop()
	sc.health.Stop()
	sc.logs.Stop()
}

// broadcastLoop consumes health/log/status updates and broadcasts via SSE.
func (sc *stackContext) broadcastLoop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	healthCh := sc.health.Output()
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
			services := sc.api.getServicesData()
			sc.sseHub.Broadcast("services", services)

			infra := sc.api.getInfraData()
			sc.sseHub.Broadcast("infrastructure", infra)

			stackStatus := sc.api.getStackStatusData()
			sc.sseHub.Broadcast("stack_status", stackStatus)

			sc.startLogStreaming()
		}
	}
}

// startLogStreaming starts tailing logs for running services and stops
// streaming for services that are no longer running.
func (sc *stackContext) startLogStreaming() {
	services := sc.api.wrapper.GetServices()

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
		if svc.Status == "running" {
			if container, ok := dockerContainerNames[svc.Name]; ok {
				if err := sc.logs.StartDocker(svc.Name, container); err != nil {
					sc.log.WithError(err).WithField(
						"service", svc.Name,
					).Warn("Failed to start Docker log streaming")
				}

				continue
			}

			if svc.LogFile != "" {
				if err := sc.logs.Start(svc.Name, svc.LogFile); err != nil {
					sc.log.WithError(err).WithField(
						"service", svc.Name,
					).Warn("Failed to start log streaming")
				}
			}

			continue
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
