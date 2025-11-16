package process

import (
	"context"
	"fmt"
	"net"
	"time"
)

// PortHealthChecker checks if a service is listening on a TCP port.
type PortHealthChecker struct {
	Host    string
	Port    int
	Timeout time.Duration
}

// NewPortHealthChecker creates a health checker for a TCP port.
func NewPortHealthChecker(host string, port int, timeout time.Duration) *PortHealthChecker {
	return &PortHealthChecker{
		Host:    host,
		Port:    port,
		Timeout: timeout,
	}
}

// Check verifies that the service is listening on the configured TCP port.
// Retries every 500ms until the timeout is reached or the context is cancelled.
func (h *PortHealthChecker) Check(ctx context.Context) error {
	deadline := time.Now().Add(h.Timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", h.Host, h.Port), 1*time.Second)
		if err == nil {
			conn.Close()

			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
			// Retry
		}
	}

	return fmt.Errorf("port %s:%d not ready after %v", h.Host, h.Port, h.Timeout)
}

// Name returns a human-readable identifier for this health check.
func (h *PortHealthChecker) Name() string {
	return fmt.Sprintf("port-%s:%d", h.Host, h.Port)
}

// NoOpHealthChecker is a health checker that always succeeds (default).
type NoOpHealthChecker struct{}

// Check always returns nil (no-op health check).
func (h *NoOpHealthChecker) Check(ctx context.Context) error {
	return nil
}

// Name returns "noop" as the identifier for this health check.
func (h *NoOpHealthChecker) Name() string {
	return "noop"
}
