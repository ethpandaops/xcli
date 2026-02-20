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
	"github.com/ethpandaops/xcli/pkg/git"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
)

// Server is the Command Center HTTP server.
type Server struct {
	log  logrus.FieldLogger
	port int
	srv  *http.Server
	wg   sync.WaitGroup

	stacks   map[string]*stackContext
	stacksMu sync.RWMutex
}

// stackInfoResponse describes an available stack for the frontend switcher.
type stackInfoResponse struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// NewServer creates a new Command Center server from the full config.
func NewServer(
	log logrus.FieldLogger,
	cfg *config.Config,
	cfgPath string,
	port int,
) (*Server, error) {
	l := log.WithField("component", "cc")
	gitChk := git.NewChecker(l)

	s := &Server{
		log:    l,
		port:   port,
		stacks: make(map[string]*stackContext, 2),
	}

	if cfg.Lab != nil {
		orch, err := orchestrator.NewOrchestrator(
			l, cfg.Lab, cfgPath,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create lab orchestrator: %w", err,
			)
		}

		s.stacks["lab"] = newStackContext(
			l, "lab", "Lab", orch, cfg.Lab, cfgPath, gitChk,
		)
	}

	if len(s.stacks) == 0 {
		return nil, fmt.Errorf(
			"no stacks configured — need at least a lab section",
		)
	}

	return s, nil
}

// Start initializes background workers and starts the HTTP server.
// If autoOpen is true, it opens the dashboard in the default browser.
func (s *Server) Start(ctx context.Context, autoOpen bool) error {
	for _, sc := range s.stacks {
		sc.Start(ctx, &s.wg)
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := fmt.Sprintf(":%d", s.port)
	s.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)

	go func() {
		s.log.WithField("addr", addr).Info(
			"Command Center started",
		)

		if err := s.srv.ListenAndServe(); err != nil &&
			err != http.ErrServerClosed {
			errCh <- err
		}
	}()

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

	<-ctx.Done()

	return s.Stop()
}

// Stop gracefully shuts down the server and background workers.
func (s *Server) Stop() error {
	s.log.Info("Shutting down Command Center")

	for _, sc := range s.stacks {
		sc.Stop()
	}

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

// handleGetStacks returns the list of available stacks dynamically
// from the server's stacks map.
func (s *Server) handleGetStacks(
	w http.ResponseWriter,
	_ *http.Request,
) {
	s.stacksMu.RLock()
	stacks := make([]stackInfoResponse, 0, len(s.stacks))

	for _, sc := range s.stacks {
		stacks = append(stacks, stackInfoResponse{
			Name:  sc.name,
			Label: sc.label,
		})
	}

	s.stacksMu.RUnlock()

	writeJSON(w, http.StatusOK, stacks)
}

// registerRoutes sets up all HTTP routes on the given mux.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	s.registerStackRoutes(mux, "/api/stacks/{stack}")
	mux.HandleFunc("GET /api/stacks", s.handleGetStacks)

	// SPA - must be last (catch-all)
	mux.Handle("/", newSPAHandler())
}

// stackHandler extracts the {stack} path parameter, looks up the
// corresponding stackContext, and dispatches to the given handler.
// Returns 404 for unknown stacks.
func (s *Server) stackHandler(
	fn func(*stackContext, http.ResponseWriter, *http.Request),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("stack")

		s.stacksMu.RLock()
		sc, ok := s.stacks[name]
		s.stacksMu.RUnlock()

		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "unknown stack: " + name,
			})

			return
		}

		fn(sc, w, r)
	}
}

// registerStackRoutes registers all stack-scoped API routes under
// the given prefix. Each handler is dispatched through stackHandler
// which resolves the {stack} path parameter.
func (s *Server) registerStackRoutes(
	mux *http.ServeMux,
	prefix string,
) {
	sh := s.stackHandler

	// Status & info
	mux.HandleFunc("GET "+prefix+"/status",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetStatus(w, r)
		}))
	mux.HandleFunc("GET "+prefix+"/services",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetServices(w, r)
		}))
	mux.HandleFunc("GET "+prefix+"/infrastructure",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetInfrastructure(w, r)
		}))
	mux.HandleFunc("GET "+prefix+"/git",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetGit(w, r)
		}))

	// Service actions
	mux.HandleFunc("POST "+prefix+"/services/{name}/start",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePostServiceAction(w, r, "start")
		}))
	mux.HandleFunc("POST "+prefix+"/services/{name}/stop",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePostServiceAction(w, r, "stop")
		}))
	mux.HandleFunc("POST "+prefix+"/services/{name}/restart",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePostServiceAction(w, r, "restart")
		}))
	mux.HandleFunc("POST "+prefix+"/services/{name}/rebuild",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePostServiceAction(w, r, "rebuild")
		}))

	// Config management
	mux.HandleFunc("GET "+prefix+"/config",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetLabConfig(w, r)
		}))
	mux.HandleFunc("PUT "+prefix+"/config",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePutLabConfig(w, r)
		}))
	mux.HandleFunc("GET "+prefix+"/config/files",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetConfigFiles(w, r)
		}))
	mux.HandleFunc("GET "+prefix+"/config/files/{name}",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetConfigFile(w, r)
		}))
	mux.HandleFunc("PUT "+prefix+"/config/files/{name}/override",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePutConfigFileOverride(w, r)
		}))
	mux.HandleFunc("DELETE "+prefix+"/config/files/{name}/override",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleDeleteConfigFileOverride(w, r)
		}))
	mux.HandleFunc("GET "+prefix+"/config/overrides",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetOverrides(w, r)
		}))
	mux.HandleFunc("PUT "+prefix+"/config/overrides",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePutOverrides(w, r)
		}))
	mux.HandleFunc("POST "+prefix+"/config/regenerate",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePostRegenerate(w, r)
		}))

	// Stack control
	mux.HandleFunc("POST "+prefix+"/stack/up",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePostStackUp(w, r)
		}))
	mux.HandleFunc("POST "+prefix+"/stack/down",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePostStackDown(w, r)
		}))
	mux.HandleFunc("POST "+prefix+"/stack/restart",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePostStackRestart(w, r)
		}))
	mux.HandleFunc("POST "+prefix+"/stack/cancel",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePostStackCancel(w, r)
		}))
	mux.HandleFunc("GET "+prefix+"/stack/status",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetStackStatus(w, r)
		}))

	// Redis explorer
	mux.HandleFunc("GET "+prefix+"/redis/status",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetRedisStatus(w, r)
		}))
	mux.HandleFunc("GET "+prefix+"/redis/tree",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetRedisTree(w, r)
		}))
	mux.HandleFunc("GET "+prefix+"/redis/keys/search",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetRedisSearch(w, r)
		}))
	mux.HandleFunc("GET "+prefix+"/redis/key",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetRedisKey(w, r)
		}))
	mux.HandleFunc("POST "+prefix+"/redis/key",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePostRedisKey(w, r)
		}))
	mux.HandleFunc("PUT "+prefix+"/redis/key",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePutRedisKey(w, r)
		}))
	mux.HandleFunc("DELETE "+prefix+"/redis/key",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleDeleteRedisKey(w, r)
		}))
	mux.HandleFunc("POST "+prefix+"/redis/keys/delete",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePostRedisDeleteMany(w, r)
		}))

	// Logs
	mux.HandleFunc("GET "+prefix+"/services/{name}/logs",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetServiceLogs(w, r)
		}))

	// Diagnosis
	mux.HandleFunc("POST "+prefix+"/services/{name}/diagnose",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handlePostDiagnose(w, r)
		}))
	mux.HandleFunc("GET "+prefix+"/diagnose/available",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.api.handleGetDiagnoseAvailable(w, r)
		}))
	mux.HandleFunc("GET "+prefix+"/logs",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.handleGetLogs(w, r)
		}))

	// SSE events
	mux.HandleFunc("GET "+prefix+"/events",
		sh(func(sc *stackContext, w http.ResponseWriter, r *http.Request) {
			sc.sseHub.ServeHTTP(w, r)
		}))
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
