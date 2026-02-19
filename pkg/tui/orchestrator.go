package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/infrastructure"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
)

// OrchestratorWrapper wraps the orchestrator for TUI access.
type OrchestratorWrapper struct {
	orch *orchestrator.Orchestrator
}

// SetOrchestrator replaces the underlying orchestrator (e.g. after config reload).
func (w *OrchestratorWrapper) SetOrchestrator(orch *orchestrator.Orchestrator) {
	w.orch = orch
}

// NewOrchestratorWrapper creates a wrapper.
func NewOrchestratorWrapper(orch *orchestrator.Orchestrator) *OrchestratorWrapper {
	return &OrchestratorWrapper{orch: orch}
}

// ServiceInfo contains service status information.
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

// InfraInfo contains infrastructure status.
type InfraInfo struct {
	Name   string
	Status string // "running", "stopped"
	Type   string // "clickhouse", "redis"
}

// GetServices returns current service statuses.
// Re-scans PID files each call to discover externally started processes.
func (w *OrchestratorWrapper) GetServices() []ServiceInfo {
	// Re-scan PID directory to pick up processes started after CC launched
	w.orch.ProcessManager().ReloadPIDs()

	validServices := w.orch.GetValidServices()
	processes := w.orch.ProcessManager().List()

	// Get observability status if enabled
	var obsStatus map[string]infrastructure.ContainerStatus

	obsStatus, _ = w.orch.InfrastructureManager().GetObservabilityStatus(context.Background())

	services := make([]ServiceInfo, 0, len(validServices))

	for _, name := range validServices {
		// Handle observability services (Docker containers) separately
		if name == constants.ServicePrometheus || name == constants.ServiceGrafana {
			info := ServiceInfo{
				Name:   name,
				Status: statusStopped,
				URL:    w.orch.GetServiceURL(name),
				Ports:  w.orch.GetServicePorts(name),
				Health: "unknown",
			}

			if status, ok := obsStatus[name]; ok {
				if status.Running {
					info.Status = statusRunning
					info.Health = healthHealthy
				}
			}

			services = append(services, info)

			continue
		}

		info := ServiceInfo{
			Name:   name,
			Status: statusStopped,
			URL:    w.orch.GetServiceURL(name),
			Ports:  w.orch.GetServicePorts(name),
			Health: "unknown",
		}

		// Find running process
		for _, proc := range processes {
			if proc.Name == name {
				info.Status = statusRunning
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

// GetInfrastructure returns infrastructure statuses in stable order.
func (w *OrchestratorWrapper) GetInfrastructure() []InfraInfo {
	infraMgr := w.orch.InfrastructureManager()
	statuses := infraMgr.Status()

	// Sort by name for stable display order.
	names := make([]string, 0, len(statuses))
	for name := range statuses {
		names = append(names, name)
	}

	sort.Strings(names)

	infra := make([]InfraInfo, 0, len(names))

	for _, name := range names {
		running := statuses[name]

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

// StartService starts a service.
func (w *OrchestratorWrapper) StartService(ctx context.Context, name string) error {
	return w.orch.StartService(ctx, name)
}

// StopService stops a service.
func (w *OrchestratorWrapper) StopService(ctx context.Context, name string) error {
	return w.orch.StopService(ctx, name)
}

// RestartService restarts a service.
// Delegates to the orchestrator's Restart which handles both process-managed
// services and Docker-based observability services (prometheus, grafana).
func (w *OrchestratorWrapper) RestartService(ctx context.Context, name string) error {
	return w.orch.Restart(ctx, name)
}

// RebuildService rebuilds the binary for a service and restarts it.
// It determines which build target to use based on the service name.
func (w *OrchestratorWrapper) RebuildService(ctx context.Context, name string) error {
	builder := w.orch.Builder()

	// Stop the service first
	_ = w.orch.StopService(ctx, name)

	// Determine which build to run based on service name
	var buildErr error

	switch {
	case name == constants.ServiceLabBackend:
		buildErr = builder.BuildLabBackend(ctx, true) // force rebuild

	case name == constants.ServiceLabFrontend:
		buildErr = builder.BuildLabFrontend(ctx)

	case strings.HasPrefix(name, constants.ServicePrefixCBTAPI):
		buildErr = builder.BuildCBTAPI(ctx, true) // force rebuild

	case strings.HasPrefix(name, constants.ServicePrefixCBT):
		buildErr = builder.BuildCBT(ctx, true) // force rebuild

	default:
		buildErr = fmt.Errorf("unknown service type for rebuild: %s", name)
	}

	if buildErr != nil {
		return fmt.Errorf("rebuild failed: %w", buildErr)
	}

	// Start the service again
	return w.orch.StartService(ctx, name)
}

// RebuildAll performs a full rebuild of the entire stack.
// This replicates the 'xcli lab rebuild all' workflow:
// 1. Regenerate xatu-cbt protos
// 2. Rebuild xatu-cbt binary
// 3. Regenerate cbt-api protos and rebuild cbt-api
// 4. Rebuild remaining binaries (cbt, lab-backend)
// 5. Regenerate configs
// 6. Restart all services
// 7. Regenerate lab-frontend types and restart.
func (w *OrchestratorWrapper) RebuildAll(ctx context.Context) error {
	builder := w.orch.Builder()

	// Step 1: Regenerate xatu-cbt protos
	if err := builder.GenerateXatuCBTProtos(ctx); err != nil {
		return fmt.Errorf("failed to regenerate xatu-cbt protos: %w", err)
	}

	// Step 2: Rebuild xatu-cbt binary
	if err := builder.BuildXatuCBT(ctx, true); err != nil {
		return fmt.Errorf("failed to rebuild xatu-cbt: %w", err)
	}

	// Step 3: Regenerate cbt-api protos and rebuild cbt-api
	if err := builder.BuildCBTAPI(ctx, true); err != nil {
		return fmt.Errorf("failed to rebuild cbt-api: %w", err)
	}

	// Step 4: Rebuild remaining binaries (cbt, lab-backend)
	if err := builder.BuildCBT(ctx, true); err != nil {
		return fmt.Errorf("failed to rebuild cbt: %w", err)
	}

	if err := builder.BuildLabBackend(ctx, true); err != nil {
		return fmt.Errorf("failed to rebuild lab-backend: %w", err)
	}

	// Step 5: Regenerate configs
	if err := w.orch.GenerateConfigs(); err != nil {
		return fmt.Errorf("failed to regenerate configs: %w", err)
	}

	// Step 6: Restart all services (if running)
	if w.orch.AreServicesRunning() {
		if err := w.orch.RestartAllServices(ctx, false); err != nil {
			return fmt.Errorf("failed to restart services: %w", err)
		}

		// Step 7: Wait for cbt-api and regenerate lab-frontend types
		if err := w.orch.WaitForCBTAPIReady(ctx); err != nil {
			return fmt.Errorf("cbt-api did not become ready: %w", err)
		}

		if err := builder.BuildLabFrontend(ctx); err != nil {
			return fmt.Errorf("failed to regenerate lab-frontend types: %w", err)
		}

		// Restart lab-frontend
		_ = w.orch.Restart(ctx, "lab-frontend")
	}

	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
