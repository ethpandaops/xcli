package cc

import (
	"context"
	"net/http"
	"time"
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
	Status          string               `json:"status"`
	RunningServices int                  `json:"runningServices"`
	TotalServices   int                  `json:"totalServices"`
	Error           string               `json:"error,omitempty"`
	Progress        []stackProgressEvent `json:"progress,omitempty"`
}

// handlePostStackUp boots the full stack via the backend.
// Runs in a background goroutine and returns immediately.
func (a *apiHandler) handlePostStackUp(w http.ResponseWriter, _ *http.Request) {
	a.stack.mu.Lock()
	if a.stack.status == stackStatusStarting || a.stack.status == stackStatusStopping ||
		a.stack.status == stackStatusRunning {
		current := a.stack.status
		a.stack.mu.Unlock()

		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "stack is currently " + current,
		})

		return
	}

	a.stack.status = stackStatusStarting
	a.stack.lastError = ""
	a.stack.progressEvents = nil
	a.stack.mu.Unlock()

	a.sseHub.Broadcast("stack_starting", nil)

	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		a.stack.mu.Lock()
		a.stack.cancelBoot = cancel
		a.stack.mu.Unlock()

		defer func() {
			a.stack.mu.Lock()
			a.stack.cancelBoot = nil
			a.stack.mu.Unlock()
		}()

		a.log.Info("starting stack from CC dashboard")

		progress := func(phase string, message string) {
			evt := stackProgressEvent{Phase: phase, Message: message}

			a.stack.mu.Lock()
			a.stack.progressEvents = append(a.stack.progressEvents, evt)
			a.stack.mu.Unlock()

			a.sseHub.Broadcast("stack_progress", map[string]string{
				"phase":   phase,
				"message": message,
			})
		}

		err := a.backend.Up(ctx, progress)

		a.stack.mu.Lock()

		if err != nil {
			a.stack.status = stackStatusIdle
			a.stack.lastError = err.Error()
		} else {
			a.stack.status = stackStatusRunning
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

// handlePostStackDown tears down the full stack via the backend.
// Runs in a background goroutine and returns immediately.
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
	a.stack.progressEvents = nil
	a.stack.mu.Unlock()

	a.sseHub.Broadcast("stack_stopping", nil)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), stackShutdownTimeout)
		defer cancel()

		a.log.Info("tearing down stack from CC dashboard")

		progress := func(phase string, message string) {
			evt := stackProgressEvent{Phase: phase, Message: message}

			a.stack.mu.Lock()
			a.stack.progressEvents = append(a.stack.progressEvents, evt)
			a.stack.mu.Unlock()

			a.sseHub.Broadcast("stack_progress", map[string]string{
				"phase":   phase,
				"message": message,
			})
		}

		err := a.backend.Down(ctx, progress)

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
	a.stack.progressEvents = nil
	a.stack.mu.Unlock()

	a.sseHub.Broadcast("stack_stopping", nil)

	go func() {
		progress := func(phase string, message string) {
			evt := stackProgressEvent{Phase: phase, Message: message}

			a.stack.mu.Lock()
			a.stack.progressEvents = append(a.stack.progressEvents, evt)
			a.stack.mu.Unlock()

			a.sseHub.Broadcast("stack_progress", map[string]string{
				"phase":   phase,
				"message": message,
			})
		}

		// Phase 1: Tear down
		a.log.Info("restarting stack from CC dashboard — tearing down")

		downCtx, downCancel := context.WithTimeout(context.Background(), stackShutdownTimeout)
		defer downCancel()

		if err := a.backend.Down(downCtx, progress); err != nil {
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

		// Phase 2: Boot
		a.log.Info("restarting stack from CC dashboard — booting")

		a.stack.mu.Lock()
		a.stack.status = stackStatusStarting
		a.stack.progressEvents = nil
		a.stack.mu.Unlock()

		a.sseHub.Broadcast("stack_starting", nil)

		upCtx, upCancel := context.WithCancel(context.Background())
		defer upCancel()

		a.stack.mu.Lock()
		a.stack.cancelBoot = upCancel
		a.stack.mu.Unlock()

		defer func() {
			a.stack.mu.Lock()
			a.stack.cancelBoot = nil
			a.stack.mu.Unlock()
		}()

		if err := a.backend.Up(upCtx, progress); err != nil {
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

// getStackStatusData builds the current stack status snapshot.
func (a *apiHandler) getStackStatusData() stackStatusResponse {
	a.stack.mu.Lock()
	currentStatus := a.stack.status
	lastErr := a.stack.lastError

	var progress []stackProgressEvent
	if len(a.stack.progressEvents) > 0 {
		progress = make([]stackProgressEvent, len(a.stack.progressEvents))
		copy(progress, a.stack.progressEvents)
	}

	a.stack.mu.Unlock()

	services := a.backend.GetServices(context.Background())
	running := 0

	for _, svc := range services {
		if svc.Status == stackStatusRunning {
			running++
		}
	}

	// Derive display status.
	// Only report "running" if the stack was explicitly booted (currentStatus == "running").
	// If currentStatus is "idle" but some services happen to be up (e.g. port conflicts
	// from another stack), report "stopped" so we don't confuse the frontend.
	status := stackStatusStopped

	switch {
	case currentStatus == stackStatusStarting:
		status = stackStatusStarting
	case currentStatus == stackStatusStopping:
		status = stackStatusStopping
	case currentStatus == stackStatusRunning:
		status = stackStatusRunning
	}

	return stackStatusResponse{
		Status:          status,
		RunningServices: running,
		TotalServices:   len(services),
		Error:           lastErr,
		Progress:        progress,
	}
}

// handleGetStackStatus returns the current stack-level status.
func (a *apiHandler) handleGetStackStatus(
	w http.ResponseWriter,
	_ *http.Request,
) {
	writeJSON(w, http.StatusOK, a.getStackStatusData())
}

// handlePostStackCancel aborts an in-progress boot and tears down any
// dangling services/infra that were already started.
func (a *apiHandler) handlePostStackCancel(
	w http.ResponseWriter,
	_ *http.Request,
) {
	a.stack.mu.Lock()

	if a.stack.status != stackStatusStarting {
		a.stack.mu.Unlock()

		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "stack is not currently starting",
		})

		return
	}

	if a.stack.cancelBoot != nil {
		a.stack.cancelBoot()
	}

	a.stack.status = stackStatusStopping
	a.stack.cancelBoot = nil
	a.stack.progressEvents = nil
	a.stack.mu.Unlock()

	a.sseHub.Broadcast("stack_stopping", nil)

	go func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), stackShutdownTimeout,
		)
		defer cancel()

		a.log.Info("cancelling boot — tearing down partial stack")

		progress := func(phase string, message string) {
			evt := stackProgressEvent{Phase: phase, Message: message}

			a.stack.mu.Lock()
			a.stack.progressEvents = append(a.stack.progressEvents, evt)
			a.stack.mu.Unlock()

			a.sseHub.Broadcast("stack_progress", map[string]string{
				"phase":   phase,
				"message": message,
			})
		}

		if err := a.backend.Down(ctx, progress); err != nil {
			a.log.WithError(err).Error("teardown after cancel failed")

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
		a.stack.lastError = ""
		a.stack.mu.Unlock()

		a.log.Info("boot cancelled and stack torn down successfully")
		a.sseHub.Broadcast("stack_stopped", nil)
	}()

	writeJSON(w, http.StatusOK, map[string]string{
		"status": stackStatusStopping,
	})
}
