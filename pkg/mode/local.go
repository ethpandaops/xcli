package mode

import (
	"github.com/ethpandaops/xcli/pkg/config"
)

// LocalMode implements fully local stack with no external dependencies.
type LocalMode struct {
	config *config.Config
}

// NewLocalMode creates a new local mode instance.
func NewLocalMode(cfg *config.Config) *LocalMode {
	return &LocalMode{config: cfg}
}

// Name returns the mode identifier "local".
func (m *LocalMode) Name() string {
	return "local"
}

// GetInfrastructureServices returns the list of docker services required for local mode.
// Local mode includes both local ClickHouse instances and Redis.
func (m *LocalMode) GetInfrastructureServices() []string {
	return []string{
		"clickhouse-cbt",
		"clickhouse-xatu-local",
		"redis",
	}
}

// GetHealthCheckPorts returns the ports to health check for infrastructure readiness.
// Local mode checks both local ClickHouse instances and Redis.
func (m *LocalMode) GetHealthCheckPorts() []int {
	return []int{
		m.config.Lab.Infrastructure.ClickHouseXatuPort, // ClickHouse Xatu (local)
		m.config.Lab.Infrastructure.ClickHouseCBTPort,  // ClickHouse CBT
		m.config.Lab.Infrastructure.RedisPort,          // Redis
	}
}

// NeedsExternalClickHouse returns false as local mode uses local ClickHouse instances.
func (m *LocalMode) NeedsExternalClickHouse() bool {
	return false
}

// GetObservabilityPorts returns ports for observability services (Prometheus, Grafana).
// Returns nil if observability is disabled.
func (m *LocalMode) GetObservabilityPorts() []int {
	if !m.config.Lab.Infrastructure.Observability.Enabled {
		return nil
	}

	return []int{
		m.config.Lab.Infrastructure.Observability.PrometheusPort,
		m.config.Lab.Infrastructure.Observability.GrafanaPort,
	}
}

// ValidateConfig validates the configuration for local mode.
// Local mode has no special validation requirements.
func (m *LocalMode) ValidateConfig(cfg *config.Config) error {
	// No special validation for local mode
	return nil
}
