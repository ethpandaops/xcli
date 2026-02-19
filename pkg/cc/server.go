package cc

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
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

// Server is the Command Center HTTP server.
type Server struct {
	log     logrus.FieldLogger
	wrapper *tui.OrchestratorWrapper
	health  *tui.HealthMonitor
	logs    *tui.LogStreamer
	sseHub  *SSEHub
	api     *apiHandler
	labCfg  *config.LabConfig
	cfgPath string
	port    int
	srv     *http.Server
	wg      sync.WaitGroup

	// logHistory is a ring buffer of recent log lines so new SSE clients
	// can catch up on logs emitted before they connected.
	logHistory   []tui.LogLine
	logHistoryMu sync.RWMutex
}

// NewServer creates a new Command Center server.
func NewServer(
	log logrus.FieldLogger,
	orch *orchestrator.Orchestrator,
	labCfg *config.LabConfig,
	cfgPath string,
	port int,
) *Server {
	l := log.WithField("component", "cc")
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
		gitChk:  git.NewChecker(l),
		sseHub:  sseHub,
	}

	return &Server{
		log:     l,
		wrapper: wrapper,
		health:  healthMon,
		logs:    logStreamer,
		sseHub:  sseHub,
		api:     api,
		labCfg:  labCfg,
		cfgPath: cfgPath,
		port:    port,
	}
}

// Start initializes background workers and starts the HTTP server.
// If autoOpen is true, it opens the dashboard in the default browser.
func (s *Server) Start(ctx context.Context, autoOpen bool) error {
	// Start health monitoring
	s.health.Start()

	// Log streaming is started by the broadcastLoop ticker (every 2s) rather
	// than here, so that SSE clients have time to connect before the initial
	// burst of Docker log history is broadcast.

	// Start SSE background broadcaster
	s.wg.Add(1)

	go s.broadcastLoop(ctx)

	// Build HTTP mux
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := fmt.Sprintf(":%d", s.port)
	s.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start server in background
	errCh := make(chan error, 1)

	go func() {
		s.log.WithField("addr", addr).Info("Command Center started")

		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Give server a moment to bind
	time.Sleep(100 * time.Millisecond)

	select {
	case err := <-errCh:
		return fmt.Errorf("failed to start server: %w", err)
	default:
	}

	url := fmt.Sprintf("http://localhost:%d", s.port)
	s.log.WithField("url", url).Info("Dashboard available")

	fmt.Printf("\n  Command Center running at: %s\n\n", url)

	if autoOpen {
		openBrowser(url)
	}

	// Wait for context cancellation
	<-ctx.Done()

	return s.Stop()
}

// Stop gracefully shuts down the server and background workers.
func (s *Server) Stop() error {
	s.log.Info("Shutting down Command Center")

	s.sseHub.Stop()
	s.health.Stop()
	s.logs.Stop()

	if s.srv != nil {
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cancel()

		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown error: %w", err)
		}
	}

	s.wg.Wait()

	return nil
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// API routes
	mux.HandleFunc("GET /api/status", s.api.handleGetStatus)
	mux.HandleFunc("GET /api/services", s.api.handleGetServices)
	mux.HandleFunc("GET /api/infrastructure", s.api.handleGetInfrastructure)
	mux.HandleFunc("GET /api/config", s.api.handleGetConfig)
	mux.HandleFunc("GET /api/git", s.api.handleGetGit)

	// Service actions
	mux.HandleFunc("POST /api/services/{name}/start", func(w http.ResponseWriter, r *http.Request) {
		s.api.handlePostServiceAction(w, r, "start")
	})
	mux.HandleFunc("POST /api/services/{name}/stop", func(w http.ResponseWriter, r *http.Request) {
		s.api.handlePostServiceAction(w, r, "stop")
	})
	mux.HandleFunc("POST /api/services/{name}/restart", func(w http.ResponseWriter, r *http.Request) {
		s.api.handlePostServiceAction(w, r, "restart")
	})
	mux.HandleFunc("POST /api/services/{name}/rebuild", func(w http.ResponseWriter, r *http.Request) {
		s.api.handlePostServiceAction(w, r, "rebuild")
	})

	// Config management
	mux.HandleFunc("GET /api/config/lab", s.api.handleGetLabConfig)
	mux.HandleFunc("PUT /api/config/lab", s.api.handlePutLabConfig)
	mux.HandleFunc("GET /api/config/files", s.api.handleGetConfigFiles)
	mux.HandleFunc("GET /api/config/files/{name}", s.api.handleGetConfigFile)
	mux.HandleFunc("PUT /api/config/files/{name}/override", s.api.handlePutConfigFileOverride)
	mux.HandleFunc("DELETE /api/config/files/{name}/override", s.api.handleDeleteConfigFileOverride)
	mux.HandleFunc("GET /api/config/overrides", s.api.handleGetOverrides)
	mux.HandleFunc("PUT /api/config/overrides", s.api.handlePutOverrides)
	mux.HandleFunc("POST /api/config/regenerate", s.api.handlePostRegenerate)

	// Stack control
	mux.HandleFunc("POST /api/stack/up", s.api.handlePostStackUp)
	mux.HandleFunc("POST /api/stack/down", s.api.handlePostStackDown)
	mux.HandleFunc("POST /api/stack/restart", s.api.handlePostStackRestart)
	mux.HandleFunc("POST /api/stack/cancel", s.api.handlePostStackCancel)
	mux.HandleFunc("GET /api/stack/status", s.api.handleGetStackStatus)

	// Logs
	mux.HandleFunc("GET /api/services/{name}/logs", s.api.handleGetServiceLogs)
	mux.HandleFunc("GET /api/logs", s.handleGetLogs)

	// SSE events
	mux.Handle("GET /api/events", s.sseHub)

	// SPA - must be last (catch-all)
	mux.Handle("/", newSPAHandler())
}

// dockerContainerNames maps observability service names to Docker container names.
var dockerContainerNames = map[string]string{
	constants.ServicePrometheus: constants.ContainerPrometheus,
	constants.ServiceGrafana:    constants.ContainerGrafana,
}

// startLogStreaming starts tailing logs for running services and stops
// streaming for services that are no longer running. This handles the case
// where tail -f processes survive log file deletion (macOS kqueue behavior)
// and must be explicitly killed so streaming can restart with fresh files.
//
// For stopped services that have a log file, it reads the last 200 lines
// once so crash logs are visible in the dashboard.
func (s *Server) startLogStreaming() {
	services := s.wrapper.GetServices()

	// Build set of currently running service names.
	running := make(map[string]bool, len(services))
	for _, svc := range services {
		if svc.Status == "running" {
			running[svc.Name] = true
		}
	}

	// Stop streaming for services that are no longer running.
	for _, name := range s.logs.ActiveServices() {
		if !running[name] {
			s.logs.StopService(name)
		}
	}

	// Start streaming for running services that aren't already tailed.
	for _, svc := range services {
		if svc.Status == "running" {
			// Docker container-based services (observability)
			if container, ok := dockerContainerNames[svc.Name]; ok {
				if err := s.logs.StartDocker(svc.Name, container); err != nil {
					s.log.WithError(err).WithField(
						"service", svc.Name,
					).Warn("Failed to start Docker log streaming")
				}

				continue
			}

			// Process-managed services (tail log file)
			if svc.LogFile != "" {
				if err := s.logs.Start(svc.Name, svc.LogFile); err != nil {
					s.log.WithError(err).WithField(
						"service", svc.Name,
					).Warn("Failed to start log streaming")
				}
			}

			continue
		}
	}
}

// broadcastLoop consumes health/log/status updates and broadcasts via SSE.
func (s *Server) broadcastLoop(ctx context.Context) {
	defer s.wg.Done()

	healthCh := s.health.Output()
	logCh := s.logs.Output()

	// Periodic service status ticker
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

			s.sseHub.Broadcast("health", health)
		case logLine, ok := <-logCh:
			if !ok {
				return
			}

			s.appendLog(logLine)
			s.sseHub.Broadcast("log", logLine)
		case <-statusTicker.C:
			services := s.api.getServicesData()
			s.sseHub.Broadcast("services", services)

			infra := s.api.getInfraData()
			s.sseHub.Broadcast("infrastructure", infra)

			stackStatus := s.api.getStackStatusData()
			s.sseHub.Broadcast("stack_status", stackStatus)

			// Start log streaming for any newly running services
			s.startLogStreaming()
		}
	}
}

// appendLog adds a log line to the history ring buffer.
func (s *Server) appendLog(line tui.LogLine) {
	s.logHistoryMu.Lock()
	defer s.logHistoryMu.Unlock()

	if len(s.logHistory) >= logHistorySize {
		s.logHistory = s.logHistory[1:]
	}

	s.logHistory = append(s.logHistory, line)
}

// handleGetLogs returns recent log history so new clients can catch up
// on logs emitted before their SSE connection was established.
func (s *Server) handleGetLogs(w http.ResponseWriter, _ *http.Request) {
	s.logHistoryMu.RLock()
	logs := make([]tui.LogLine, len(s.logHistory))
	copy(logs, s.logHistory)
	s.logHistoryMu.RUnlock()

	writeJSON(w, http.StatusOK, logs)
}

// openBrowser opens the given URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}

	_ = cmd.Start()
}
