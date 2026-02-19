package cc

import (
	"context"
	"net/http"
	"time"

	"github.com/ethpandaops/xcli/pkg/orchestrator"
)

const (
	// stackShutdownTimeout is the maximum time to wait for the stack to stop.
	stackShutdownTimeout = 5 * time.Minute

	stackStatusStarting = "starting"
	stackStatusStopping = "stopping"
	stackStatusRunning  = "running"
	stackStatusStopped  = "stopped"
	stackStatusIdle     = "idle"
)

// stackStatusResponse is the JSON response for GET /api/stack/status.
type stackStatusResponse struct {
	Status          string `json:"status"`
	RunningServices int    `json:"runningServices"`
	TotalServices   int    `json:"totalServices"`
	Error           string `json:"error,omitempty"`
}

// handlePostStackUp boots the full stack (equivalent to `xcli lab up`).
// Runs the orchestrator's Up() in a background goroutine and returns immediately.
func (a *apiHandler) handlePostStackUp(w http.ResponseWriter, _ *http.Request) {
	a.stack.mu.Lock()
	if a.stack.status == stackStatusStarting || a.stack.status == stackStatusStopping {
		current := a.stack.status
		a.stack.mu.Unlock()

		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "stack is currently " + current,
		})

		return
	}

	// Check if services are already running
	services := a.wrapper.GetServices()
	running := 0

	for _, svc := range services {
		if svc.Status == stackStatusRunning {
			running++
		}
	}

	if running > 0 {
		a.stack.mu.Unlock()

		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "stack is already running",
		})

		return
	}

	a.stack.status = stackStatusStarting
	a.stack.lastError = ""
	a.stack.mu.Unlock()

	a.sseHub.Broadcast("stack_starting", nil)

	// Run Up() in a background goroutine — terminal UI output goes to server stdout
	go func() {
		ctx := context.Background()

		a.log.Info("starting stack from CC dashboard")

		progress := orchestrator.ProgressFunc(func(phase string, message string) {
			a.sseHub.Broadcast("stack_progress", map[string]string{
				"phase":   phase,
				"message": message,
			})
		})

		err := a.orch.Up(ctx, false, false, progress)

		a.stack.mu.Lock()
		a.stack.status = stackStatusIdle

		if err != nil {
			a.stack.lastError = err.Error()
		}

		a.stack.mu.Unlock()

		if err != nil {
			a.log.WithError(err).Error("stack boot failed")
			a.sseHub.Broadcast("stack_error", map[string]string{
				"error": err.Error(),
			})

			return
		}

		a.log.Info("stack boot completed successfully")
		a.sseHub.Broadcast("stack_started", nil)
	}()

	writeJSON(w, http.StatusOK, map[string]string{
		"status": stackStatusStarting,
	})
}

// handlePostStackDown tears down the full stack (equivalent to `xcli lab down`).
// Runs the orchestrator's Down() in a background goroutine and returns immediately.
func (a *apiHandler) handlePostStackDown(
	w http.ResponseWriter,
	_ *http.Request,
) {
	a.stack.mu.Lock()
	if a.stack.status == stackStatusStarting || a.stack.status == stackStatusStopping {
		current := a.stack.status
		a.stack.mu.Unlock()

		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "stack is currently " + current,
		})

		return
	}

	a.stack.status = stackStatusStopping
	a.stack.lastError = ""
	a.stack.mu.Unlock()

	a.sseHub.Broadcast("stack_stopping", nil)

	// Run Down() in a background goroutine so it doesn't depend on the HTTP
	// request lifecycle. Same pattern as handlePostStackUp.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), stackShutdownTimeout)
		defer cancel()

		a.log.Info("tearing down stack from CC dashboard")

		progress := orchestrator.ProgressFunc(func(phase string, message string) {
			a.sseHub.Broadcast("stack_progress", map[string]string{
				"phase":   phase,
				"message": message,
			})
		})

		err := a.orch.Down(ctx, progress)

		a.stack.mu.Lock()
		a.stack.status = stackStatusIdle

		if err != nil {
			a.stack.lastError = err.Error()
		}

		a.stack.mu.Unlock()

		if err != nil {
			a.log.WithError(err).Error("stack teardown failed")
			a.sseHub.Broadcast("stack_error", map[string]string{
				"error": err.Error(),
			})

			return
		}

		a.log.Info("stack torn down successfully")
		a.sseHub.Broadcast("stack_stopped", nil)
	}()

	writeJSON(w, http.StatusOK, map[string]string{
		"status": stackStatusStopping,
	})
}

// handlePostStackRestart tears down the stack and boots it again.
// Runs down→up sequentially in a single background goroutine.
func (a *apiHandler) handlePostStackRestart(
	w http.ResponseWriter,
	_ *http.Request,
) {
	a.stack.mu.Lock()
	if a.stack.status == stackStatusStarting || a.stack.status == stackStatusStopping {
		current := a.stack.status
		a.stack.mu.Unlock()

		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "stack is currently " + current,
		})

		return
	}

	a.stack.status = stackStatusStopping
	a.stack.lastError = ""
	a.stack.mu.Unlock()

	a.sseHub.Broadcast("stack_stopping", nil)

	go func() {
		progress := orchestrator.ProgressFunc(func(phase string, message string) {
			a.sseHub.Broadcast("stack_progress", map[string]string{
				"phase":   phase,
				"message": message,
			})
		})

		// Phase 1: Tear down
		a.log.Info("restarting stack from CC dashboard — tearing down")

		downCtx, downCancel := context.WithTimeout(context.Background(), stackShutdownTimeout)
		defer downCancel()

		if err := a.orch.Down(downCtx, progress); err != nil {
			a.log.WithError(err).Error("stack teardown failed during restart")

			a.stack.mu.Lock()
			a.stack.status = stackStatusIdle
			a.stack.lastError = err.Error()
			a.stack.mu.Unlock()

			a.sseHub.Broadcast("stack_error", map[string]string{
				"error": err.Error(),
			})

			return
		}

		// Phase 2: Boot — skip "stack_stopped" broadcast to avoid a flash
		// of "Stack is not running" in the dashboard between down→up.
		a.log.Info("restarting stack from CC dashboard — booting")

		a.stack.mu.Lock()
		a.stack.status = stackStatusStarting
		a.stack.mu.Unlock()

		a.sseHub.Broadcast("stack_starting", nil)

		upCtx := context.Background()

		if err := a.orch.Up(upCtx, false, false, progress); err != nil {
			a.log.WithError(err).Error("stack boot failed during restart")

			a.stack.mu.Lock()
			a.stack.status = stackStatusIdle
			a.stack.lastError = err.Error()
			a.stack.mu.Unlock()

			a.sseHub.Broadcast("stack_error", map[string]string{
				"error": err.Error(),
			})

			return
		}

		a.stack.mu.Lock()
		a.stack.status = stackStatusIdle
		a.stack.mu.Unlock()

		a.log.Info("stack restart completed successfully")
		a.sseHub.Broadcast("stack_started", nil)
	}()

	writeJSON(w, http.StatusOK, map[string]string{
		"status": stackStatusStopping,
	})
}

// handleGetStackStatus returns the current stack-level status.
func (a *apiHandler) handleGetStackStatus(
	w http.ResponseWriter,
	_ *http.Request,
) {
	a.stack.mu.Lock()
	currentStatus := a.stack.status
	lastErr := a.stack.lastError
	a.stack.mu.Unlock()

	services := a.wrapper.GetServices()
	running := 0

	for _, svc := range services {
		if svc.Status == stackStatusRunning {
			running++
		}
	}

	// Derive display status
	status := stackStatusStopped

	switch {
	case currentStatus == stackStatusStarting:
		status = stackStatusStarting
	case currentStatus == stackStatusStopping:
		status = stackStatusStopping
	case running > 0:
		status = stackStatusRunning
	}

	writeJSON(w, http.StatusOK, stackStatusResponse{
		Status:          status,
		RunningServices: running,
		TotalServices:   len(services),
		Error:           lastErr,
	})
}
