package tui

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HealthStatus represents health check result.
type HealthStatus struct {
	Status              string // "healthy", "unhealthy", "degraded", "unknown"
	LastCheck           time.Time
	LastError           string
	ConsecutiveFailures int
}

// HealthMonitor monitors service health.
type HealthMonitor struct {
	services map[string]HealthStatus
	wrapper  *OrchestratorWrapper
	cancel   context.CancelFunc
	mu       sync.RWMutex
	output   chan map[string]HealthStatus
}

// NewHealthMonitor creates a health monitor.
func NewHealthMonitor(wrapper *OrchestratorWrapper) *HealthMonitor {
	return &HealthMonitor{
		services: make(map[string]HealthStatus, 10),
		wrapper:  wrapper,
		output:   make(chan map[string]HealthStatus, 10),
	}
}

// Start begins health monitoring.
func (hm *HealthMonitor) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	hm.cancel = cancel

	go hm.monitor(ctx)
}

// Output returns the health status channel.
func (hm *HealthMonitor) Output() <-chan map[string]HealthStatus {
	return hm.output
}

// Stop stops health monitoring.
func (hm *HealthMonitor) Stop() {
	hm.cancel()
	close(hm.output)
}

func (hm *HealthMonitor) monitor(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hm.checkAll()
		}
	}
}

func (hm *HealthMonitor) checkAll() {
	services := hm.wrapper.GetServices()

	hm.mu.Lock()
	defer hm.mu.Unlock()

	for _, svc := range services {
		if svc.Status != statusRunning {
			hm.services[svc.Name] = HealthStatus{
				Status:    "unknown",
				LastCheck: time.Now(),
			}

			continue
		}

		status := hm.checkService(svc)
		hm.services[svc.Name] = status
	}

	// Send update
	snapshot := make(map[string]HealthStatus, len(hm.services))
	for k, v := range hm.services {
		snapshot[k] = v
	}

	select {
	case hm.output <- snapshot:
	default:
	}
}

func (hm *HealthMonitor) checkService(svc ServiceInfo) HealthStatus {
	status := HealthStatus{
		LastCheck: time.Now(),
		Status:    "unknown",
	}

	// Determine check method based on service type
	var err error

	if strings.HasPrefix(svc.Name, "cbt-api-") {
		// Has /health endpoint
		healthURL := fmt.Sprintf("%s/health", svc.URL)
		err = checkHTTP(healthURL)
	} else if len(svc.Ports) > 0 {
		// Port check
		err = checkPort(svc.Ports[0])
	}

	if err != nil {
		prev := hm.services[svc.Name]
		status.Status = healthUnhealthy
		status.LastError = err.Error()
		status.ConsecutiveFailures = prev.ConsecutiveFailures + 1
	} else {
		status.Status = healthHealthy
		status.ConsecutiveFailures = 0
	}

	return status
}

func checkHTTP(url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // URL is constructed from local service config
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return nil
}

func checkPort(port int) error {
	addr := fmt.Sprintf("localhost:%d", port)

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}

	conn.Close()

	return nil
}
