package infrastructure

import (
	"path/filepath"
	"testing"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/instance"
	"github.com/stretchr/testify/require"
)

func TestObservabilityResourcesUseRuntimeDockerPlanAndPorts(t *testing.T) {
	cfg := testObservabilityConfig()
	configPath := filepath.Join(t.TempDir(), config.DefaultConfigFileName)
	dockerPlan := instance.NewDockerPlan("alpha", configPath)
	runtime := &instance.Runtime{
		Docker: dockerPlan,
		Manifest: &instance.Manifest{
			Docker:   dockerPlan,
			StateDir: filepath.Join(t.TempDir(), ".xcli", "instances", "alpha"),
		},
		Ports: instance.PortPlan{
			Prometheus: 19090,
			Grafana:    13000,
		},
	}

	resources := newObservabilityResources(cfg, runtime)
	require.Equal(t, "xcli-alpha-prometheus", resources.containers[constants.ServicePrometheus])
	require.Equal(t, "xcli-alpha-grafana", resources.containers[constants.ServiceGrafana])
	require.Equal(t, "xcli-alpha-prometheus-data", resources.volumes[constants.ServicePrometheus])
	require.Equal(t, "xcli-alpha-grafana-data", resources.volumes[constants.ServiceGrafana])
	require.Equal(t, 19090, resources.ports[constants.ServicePrometheus])
	require.Equal(t, 13000, resources.ports[constants.ServiceGrafana])
	require.Equal(t, "alpha", resources.labels["com.ethpandaops.xcli.instance"])
	require.Equal(t, configPath, resources.labels["com.ethpandaops.xcli.config"])

	manager := &ObservabilityManager{cfg: cfg, resources: resources}
	require.Equal(t, "xcli-alpha-prometheus", manager.containerName(constants.ServicePrometheus))
	require.Equal(t, "xcli-alpha-prometheus-data", manager.volumeName(constants.ServicePrometheus))
	require.Equal(t, 19090, manager.servicePort(constants.ServicePrometheus))

	labels := manager.labels()
	labels["com.ethpandaops.xcli.instance"] = "mutated"
	require.Equal(t, "alpha", manager.labels()["com.ethpandaops.xcli.instance"])
}

func TestInfrastructureManagerExposesRuntimeDockerContainerName(t *testing.T) {
	cfg := testObservabilityConfig()
	configPath := filepath.Join(t.TempDir(), config.DefaultConfigFileName)
	dockerPlan := instance.NewDockerPlan("beta", configPath)
	runtime := &instance.Runtime{Docker: dockerPlan}
	manager := &Manager{cfg: cfg, runtime: runtime}

	name, ok := manager.DockerContainerName(constants.ServiceGrafana)
	require.True(t, ok)
	require.Equal(t, "xcli-beta-grafana", name)

	_, ok = manager.DockerContainerName(constants.ServiceLabBackend)
	require.False(t, ok)
}

func testObservabilityConfig() *config.LabConfig {
	return &config.LabConfig{
		Infrastructure: config.InfrastructureConfig{
			Observability: config.ObservabilityConfig{
				Enabled:        true,
				PrometheusPort: 9090,
				GrafanaPort:    3000,
			},
		},
	}
}
