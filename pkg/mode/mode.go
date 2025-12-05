// Package mode defines operational modes (local vs hybrid) for the lab stack,
// determining which infrastructure services to use and how they're configured.
package mode

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
)

// Mode defines the interface for local vs hybrid stack operation.
type Mode interface {
	// Name returns "local" or "hybrid"
	Name() string

	// GetInfrastructureServices returns the list of docker services to start
	// Local: clickhouse-cbt, clickhouse-xatu-local, redis
	// Hybrid: clickhouse-cbt, redis (no xatu-local)
	GetInfrastructureServices() []string

	// GetHealthCheckPorts returns ports to health check for infrastructure readiness
	// Local: [8123 (xatu-local), 8124 (cbt), 6380 (redis)]
	// Hybrid: [8124 (cbt), 6380 (redis), <external-clickhouse-port>]
	GetHealthCheckPorts() []int

	// GetObservabilityPorts returns ports for observability services (Prometheus, Grafana)
	// Returns nil if observability is disabled
	GetObservabilityPorts() []int

	// NeedsExternalClickHouse returns true if hybrid mode
	NeedsExternalClickHouse() bool

	// ValidateConfig validates mode-specific config requirements
	// Hybrid: ensures external_clickhouse.host is set
	// Local: no special validation
	ValidateConfig(cfg *config.Config) error
}

// NewMode creates appropriate mode based on config.
func NewMode(cfg *config.Config) (Mode, error) {
	switch cfg.Lab.Mode {
	case "local":
		return NewLocalMode(cfg), nil
	case "hybrid":
		return NewHybridMode(cfg), nil
	default:
		return nil, fmt.Errorf("unknown mode: %s (expected 'local' or 'hybrid')", cfg.Lab.Mode)
	}
}
