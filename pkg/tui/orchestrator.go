package tui

import (
	"context"
	"time"

	"github.com/ethpandaops/xcli/pkg/orchestrator"
)

// OrchestratorWrapper wraps the orchestrator for TUI access
type OrchestratorWrapper struct {
	orch *orchestrator.Orchestrator
}

// NewOrchestratorWrapper creates a wrapper
func NewOrchestratorWrapper(orch *orchestrator.Orchestrator) *OrchestratorWrapper {
	return &OrchestratorWrapper{orch: orch}
}

// ServiceInfo contains service status information
type ServiceInfo struct {
	Name    string
	Status  string // "running", "stopped", "crashed"
	PID     int
	Uptime  time.Duration
	URL     string
	Ports   []int
	Health  string // "healthy", "unhealthy", "unknown"
	LogFile string
}

// InfraInfo contains infrastructure status
type InfraInfo struct {
	Name   string
	Status string // "running", "stopped"
	Type   string // "clickhouse", "redis"
}

// GetServices returns current service statuses
func (w *OrchestratorWrapper) GetServices() []ServiceInfo {
	validServices := w.orch.GetValidServices()
	processes := w.orch.ProcessManager().List()

	services := make([]ServiceInfo, 0, len(validServices))

	for _, name := range validServices {
		info := ServiceInfo{
			Name:   name,
			Status: "stopped",
			URL:    w.orch.GetServiceURL(name),
			Ports:  w.orch.GetServicePorts(name),
			Health: "unknown",
		}

		// Find running process
		for _, proc := range processes {
			if proc.Name == name {
				info.Status = "running"
				info.PID = proc.PID
				info.Uptime = time.Since(proc.Started)
				info.LogFile = proc.LogFile

				break
			}
		}

		services = append(services, info)
	}

	return services
}

// GetInfrastructure returns infrastructure statuses
func (w *OrchestratorWrapper) GetInfrastructure() []InfraInfo {
	infraMgr := w.orch.InfrastructureManager()
	statuses := infraMgr.Status()

	infra := make([]InfraInfo, 0, len(statuses))

	for name, running := range statuses {
		status := "stopped"
		if running {
			status = "running"
		}

		infraType := "unknown"
		if contains(name, "ClickHouse") {
			infraType = "clickhouse"
		} else if contains(name, "Redis") {
			infraType = "redis"
		}

		infra = append(infra, InfraInfo{
			Name:   name,
			Status: status,
			Type:   infraType,
		})
	}

	return infra
}

// StartService starts a service
func (w *OrchestratorWrapper) StartService(ctx context.Context, name string) error {
	return w.orch.StartService(ctx, name)
}

// StopService stops a service
func (w *OrchestratorWrapper) StopService(ctx context.Context, name string) error {
	return w.orch.StopService(ctx, name)
}

// RestartService restarts a service.
func (w *OrchestratorWrapper) RestartService(ctx context.Context, name string) error {
	if err := w.orch.StopService(ctx, name); err != nil {
		return err
	}

	return w.orch.StartService(ctx, name)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
