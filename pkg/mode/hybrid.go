package mode

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
)

// HybridMode implements hybrid stack with external ClickHouse.
type HybridMode struct {
	config *config.Config
}

// NewHybridMode creates a new hybrid mode instance.
func NewHybridMode(cfg *config.Config) *HybridMode {
	return &HybridMode{config: cfg}
}

// Name returns the mode identifier "hybrid".
func (m *HybridMode) Name() string {
	return "hybrid"
}

// GetInfrastructureServices returns the list of docker services required for hybrid mode.
// Hybrid mode excludes clickhouse-xatu-local as it uses an external ClickHouse instance.
func (m *HybridMode) GetInfrastructureServices() []string {
	// No clickhouse-xatu-local in hybrid mode (external)
	return []string{
		"clickhouse-cbt",
		"redis",
	}
}

// GetHealthCheckPorts returns the ports to health check for infrastructure readiness.
// Hybrid mode only checks local ClickHouse CBT and Redis (external Xatu is assumed ready).
func (m *HybridMode) GetHealthCheckPorts() []int {
	return []int{
		m.config.Lab.Infrastructure.ClickHouseCBTPort, // ClickHouse CBT
		m.config.Lab.Infrastructure.RedisPort,         // Redis
	}
}

// NeedsExternalClickHouse returns true as hybrid mode requires an external ClickHouse connection.
func (m *HybridMode) NeedsExternalClickHouse() bool {
	return true
}

// ValidateConfig validates that hybrid mode has the required external ClickHouse configuration.
// Returns an error if the external ClickHouse mode or URL is not properly configured.
func (m *HybridMode) ValidateConfig(cfg *config.Config) error {
	if cfg.Lab.Infrastructure.ClickHouse.Xatu.Mode != "external" {
		return fmt.Errorf("hybrid mode requires infrastructure.clickhouse.xatu.mode to be 'external'")
	}

	if cfg.Lab.Infrastructure.ClickHouse.Xatu.ExternalURL == "" {
		return fmt.Errorf("hybrid mode requires infrastructure.clickhouse.xatu.externalUrl to be set")
	}

	return nil
}
