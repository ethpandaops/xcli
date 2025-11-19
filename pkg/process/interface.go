package process

import (
	"context"
	"os/exec"
)

// ProcessManager defines the interface for process lifecycle management.
// This interface enables dependency injection and testing via mocks.
type ProcessManager interface {
	// Start starts a new process with optional health checking.
	// If healthCheck is provided, waits for service to be healthy before returning.
	Start(ctx context.Context, name string, cmd *exec.Cmd, healthCheck HealthChecker) error

	// Stop stops a running process gracefully (SIGTERM then SIGKILL).
	// The context allows cancellation of the graceful shutdown wait.
	Stop(ctx context.Context, name string) error

	// StopAll stops all managed processes.
	// The context allows cancellation of the shutdown sequence.
	StopAll(ctx context.Context) error

	// Restart restarts a process (stops then starts with same command).
	Restart(ctx context.Context, name string) error

	// List returns all currently managed processes.
	List() []*Process

	// Get returns a specific process by name.
	Get(name string) (*Process, bool)

	// IsRunning checks if a process is running.
	IsRunning(name string) bool

	// TailLogs tails logs for a process.
	TailLogs(ctx context.Context, name string, follow bool) error

	// CleanLogs removes all log files.
	CleanLogs() error
}

// HealthChecker defines the interface for service health checking.
type HealthChecker interface {
	// Check verifies that a service is healthy and ready.
	// Should return nil if healthy, error otherwise.
	Check(ctx context.Context) error

	// Name returns a human-readable name for this health check.
	Name() string
}
