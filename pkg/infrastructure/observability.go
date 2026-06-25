// Package infrastructure manages Docker-based infrastructure services (ClickHouse, Redis)
// via xatu-cbt, including health checks, migrations, and mode-specific configuration.
package infrastructure

import (
	"context"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/instance"
	"github.com/ethpandaops/xcli/pkg/ui"
)

const (
	// observabilityReadyTimeout is the maximum time to wait for observability services.
	observabilityReadyTimeout = 60 * time.Second
)

// ContainerStatus represents the status of a Docker container.
type ContainerStatus struct {
	Name    string
	State   string
	Running bool
	Port    int
}

// ObservabilityManager handles Prometheus and Grafana containers.
type ObservabilityManager struct {
	log       logrus.FieldLogger
	docker    *client.Client
	cfg       *config.LabConfig
	xcliDir   string
	resources observabilityResources
}

type observabilityResources struct {
	containers map[string]string
	volumes    map[string]string
	labels     map[string]string
	ports      map[string]int
}

// NewObservabilityManager creates a new observability manager.
func NewObservabilityManager(
	log logrus.FieldLogger,
	cfg *config.LabConfig,
	xcliDir string,
) (*ObservabilityManager, error) {
	return NewObservabilityManagerWithRuntime(log, cfg, xcliDir, nil)
}

// NewObservabilityManagerWithRuntime creates a manager using instance-scoped
// Docker resources when a runtime is supplied.
func NewObservabilityManagerWithRuntime(
	log logrus.FieldLogger,
	cfg *config.LabConfig,
	xcliDir string,
	runtime *instance.Runtime,
) (*ObservabilityManager, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	if runtime != nil && runtime.Manifest != nil && runtime.Manifest.StateDir != "" {
		xcliDir = runtime.Manifest.StateDir
	}

	return &ObservabilityManager{
		log:       log.WithField("component", "observability"),
		docker:    dockerClient,
		cfg:       cfg,
		xcliDir:   xcliDir,
		resources: newObservabilityResources(cfg, runtime),
	}, nil
}

func newObservabilityResources(
	cfg *config.LabConfig,
	runtime *instance.Runtime,
) observabilityResources {
	resources := observabilityResources{
		containers: map[string]string{},
		volumes:    map[string]string{},
		labels:     map[string]string{},
		ports: map[string]int{
			constants.ServicePrometheus: cfg.Infrastructure.Observability.PrometheusPort,
			constants.ServiceGrafana:    cfg.Infrastructure.Observability.GrafanaPort,
		},
	}

	if runtime == nil {
		return resources
	}

	dockerPlan := runtime.EffectiveDockerPlan()

	for service, name := range dockerPlan.Containers {
		if name != "" {
			resources.containers[service] = name
		}
	}

	for service, name := range dockerPlan.Volumes {
		if name != "" {
			resources.volumes[service] = name
		}
	}

	resources.labels = copyStringMap(dockerPlan.Labels)

	if runtime.Ports.Prometheus != 0 {
		resources.ports[constants.ServicePrometheus] = runtime.Ports.Prometheus
	}

	if runtime.Ports.Grafana != 0 {
		resources.ports[constants.ServiceGrafana] = runtime.Ports.Grafana
	}

	return resources
}

func copyStringMap(values map[string]string) map[string]string {
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}

	return copied
}

func (m *ObservabilityManager) containerName(service string) string {
	return m.resources.containers[service]
}

func (m *ObservabilityManager) volumeName(service string) string {
	return m.resources.volumes[service]
}

func (m *ObservabilityManager) servicePort(service string) int {
	return m.resources.ports[service]
}

func (m *ObservabilityManager) labels() map[string]string {
	return copyStringMap(m.resources.labels)
}

// Start starts Prometheus and Grafana containers.
func (m *ObservabilityManager) Start(ctx context.Context) error {
	if !m.cfg.Infrastructure.Observability.Enabled {
		return nil
	}

	m.log.Info("starting observability stack")

	spinner := ui.NewSpinner("Starting observability services")

	// Start Prometheus
	spinner.UpdateText("Starting Prometheus")

	if err := m.startPrometheus(ctx); err != nil {
		spinner.Fail("Failed to start Prometheus")

		return fmt.Errorf("failed to start Prometheus: %w", err)
	}

	// Start Grafana
	spinner.UpdateText("Starting Grafana")

	if err := m.startGrafana(ctx); err != nil {
		spinner.Fail("Failed to start Grafana")

		return fmt.Errorf("failed to start Grafana: %w", err)
	}

	// Wait for services to be healthy
	spinner.UpdateText("Waiting for observability services to be healthy")

	if err := m.waitForHealth(ctx, observabilityReadyTimeout); err != nil {
		spinner.Fail("Observability services failed health check")

		return fmt.Errorf("observability health check failed: %w", err)
	}

	spinner.Success("Observability services are healthy")

	promPort := m.servicePort(constants.ServicePrometheus)
	grafanaPort := m.servicePort(constants.ServiceGrafana)

	m.log.WithFields(logrus.Fields{
		"prometheus_url": fmt.Sprintf("http://localhost:%d", promPort),
		"grafana_url":    fmt.Sprintf("http://localhost:%d", grafanaPort),
	}).Info("observability stack started")

	return nil
}

// Stop stops and removes observability containers while preserving volumes.
func (m *ObservabilityManager) Stop(ctx context.Context) error {
	if !m.cfg.Infrastructure.Observability.Enabled {
		return nil
	}

	m.log.Info("stopping observability stack")

	// Stop Grafana first
	if err := m.stopContainer(ctx, m.containerName(constants.ServiceGrafana)); err != nil {
		m.log.WithError(err).Warn("failed to stop Grafana container")
	}

	// Stop Prometheus
	if err := m.stopContainer(ctx, m.containerName(constants.ServicePrometheus)); err != nil {
		m.log.WithError(err).Warn("failed to stop Prometheus container")
	}

	m.log.Info("observability stack stopped")

	return nil
}

// Destroy stops observability containers and removes their volumes.
func (m *ObservabilityManager) Destroy(ctx context.Context) error {
	if err := m.Stop(ctx); err != nil {
		return err
	}

	if !m.cfg.Infrastructure.Observability.Enabled {
		return nil
	}

	m.log.Debug("removing observability volumes")

	if err := m.removeVolume(ctx, m.volumeName(constants.ServicePrometheus)); err != nil {
		m.log.WithError(err).Warn("failed to remove Prometheus volume")
	}

	if err := m.removeVolume(ctx, m.volumeName(constants.ServiceGrafana)); err != nil {
		m.log.WithError(err).Warn("failed to remove Grafana volume")
	}

	return nil
}

// StartService starts a specific observability container.
func (m *ObservabilityManager) StartService(ctx context.Context, service string) error {
	switch service {
	case constants.ServicePrometheus:
		m.log.Info("starting Prometheus")

		return m.startPrometheus(ctx)

	case constants.ServiceGrafana:
		m.log.Info("starting Grafana")

		return m.startGrafana(ctx)

	default:
		return fmt.Errorf("unknown observability service: %s", service)
	}
}

// StopService stops a specific observability container.
func (m *ObservabilityManager) StopService(ctx context.Context, service string) error {
	switch service {
	case constants.ServicePrometheus:
		m.log.Info("stopping Prometheus")

		return m.stopContainer(ctx, m.containerName(service))

	case constants.ServiceGrafana:
		m.log.Info("stopping Grafana")

		return m.stopContainer(ctx, m.containerName(service))

	default:
		return fmt.Errorf("unknown observability service: %s", service)
	}
}

// RestartService restarts a specific observability service (prometheus or grafana).
func (m *ObservabilityManager) RestartService(ctx context.Context, service string) error {
	switch service {
	case constants.ServicePrometheus:
		m.log.Info("restarting Prometheus")

		if err := m.stopContainer(ctx, m.containerName(service)); err != nil {
			m.log.WithError(err).Debug("failed to stop Prometheus container")
		}

		return m.startPrometheus(ctx)

	case constants.ServiceGrafana:
		m.log.Info("restarting Grafana")

		if err := m.stopContainer(ctx, m.containerName(service)); err != nil {
			m.log.WithError(err).Debug("failed to stop Grafana container")
		}

		return m.startGrafana(ctx)

	default:
		return fmt.Errorf("unknown observability service: %s", service)
	}
}

// Status returns the status of observability containers.
func (m *ObservabilityManager) Status(ctx context.Context) (map[string]ContainerStatus, error) {
	status := make(map[string]ContainerStatus, 2)

	// Check Prometheus
	promStatus, err := m.getContainerStatus(ctx, constants.ServicePrometheus)
	if err != nil {
		promStatus = ContainerStatus{
			Name:    constants.ServicePrometheus,
			State:   "not found",
			Running: false,
			Port:    m.servicePort(constants.ServicePrometheus),
		}
	}

	status[constants.ServicePrometheus] = promStatus

	// Check Grafana
	grafanaStatus, err := m.getContainerStatus(ctx, constants.ServiceGrafana)
	if err != nil {
		grafanaStatus = ContainerStatus{
			Name:    constants.ServiceGrafana,
			State:   "not found",
			Running: false,
			Port:    m.servicePort(constants.ServiceGrafana),
		}
	}

	status[constants.ServiceGrafana] = grafanaStatus

	return status, nil
}

// Close closes the Docker client.
func (m *ObservabilityManager) Close() error {
	if m.docker != nil {
		return m.docker.Close()
	}

	return nil
}

// startPrometheus starts the Prometheus container.
func (m *ObservabilityManager) startPrometheus(ctx context.Context) error {
	containerName := m.containerName(constants.ServicePrometheus)
	volumeName := m.volumeName(constants.ServicePrometheus)

	// Check if container already exists and is running
	if running, err := m.isContainerRunning(ctx, containerName); err == nil && running {
		m.log.Debug("Prometheus container already running")

		return nil
	}

	// Remove existing container if it exists (might be stopped)
	if err := m.stopContainer(ctx, containerName); err != nil {
		m.log.WithError(err).Debug("failed to remove existing Prometheus container")
	}

	// Pull image if needed
	if err := m.pullImageIfNeeded(ctx, constants.PrometheusImage); err != nil {
		return fmt.Errorf("failed to pull Prometheus image: %w", err)
	}

	// Prepare config path
	configPath := filepath.Join(m.xcliDir, "configs", "prometheus.yml")

	promPort := m.servicePort(constants.ServicePrometheus)
	if err := m.ensureVolume(ctx, volumeName); err != nil {
		return fmt.Errorf("failed to create Prometheus volume: %w", err)
	}

	// Create container
	containerConfig := &container.Config{
		Image: constants.PrometheusImage,
		Cmd: []string{
			"--config.file=/etc/prometheus/prometheus.yml",
			"--storage.tsdb.path=/prometheus",
			"--web.enable-lifecycle",
		},
		ExposedPorts: nat.PortSet{
			"9090/tcp": struct{}{},
		},
		Labels: m.labels(),
	}

	hostConfig := &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:/etc/prometheus/prometheus.yml:ro", configPath),
		},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: volumeName,
				Target: "/prometheus",
			},
		},
		PortBindings: nat.PortMap{
			"9090/tcp": []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: fmt.Sprintf("%d", promPort),
				},
			},
		},
		ExtraHosts: []string{
			"host.docker.internal:host-gateway",
		},
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
	}

	networkConfig := &network.NetworkingConfig{}

	resp, err := m.docker.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, containerName)
	if err != nil {
		return fmt.Errorf("failed to create Prometheus container: %w", err)
	}

	// Start container
	if err := m.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start Prometheus container: %w", err)
	}

	m.log.WithField("container_id", resp.ID[:12]).Debug("started Prometheus container")

	return nil
}

// startGrafana starts the Grafana container.
func (m *ObservabilityManager) startGrafana(ctx context.Context) error {
	containerName := m.containerName(constants.ServiceGrafana)
	volumeName := m.volumeName(constants.ServiceGrafana)

	// Check if container already exists and is running
	if running, err := m.isContainerRunning(ctx, containerName); err == nil && running {
		m.log.Debug("Grafana container already running")

		return nil
	}

	// Remove existing container if it exists (might be stopped)
	if err := m.stopContainer(ctx, containerName); err != nil {
		m.log.WithError(err).Debug("failed to remove existing Grafana container")
	}

	// Pull image if needed
	if err := m.pullImageIfNeeded(ctx, constants.GrafanaImage); err != nil {
		return fmt.Errorf("failed to pull Grafana image: %w", err)
	}

	// Prepare paths
	provisioningPath := filepath.Join(m.xcliDir, "configs", "grafana", "provisioning")
	dashboardsPath := filepath.Join(m.xcliDir, "configs", "grafana", "dashboards")

	grafanaPort := m.servicePort(constants.ServiceGrafana)
	if err := m.ensureVolume(ctx, volumeName); err != nil {
		return fmt.Errorf("failed to create Grafana volume: %w", err)
	}

	// Create container
	containerConfig := &container.Config{
		Image: constants.GrafanaImage,
		Env: []string{
			"GF_AUTH_ANONYMOUS_ENABLED=true",
			"GF_AUTH_ANONYMOUS_ORG_ROLE=Admin",
			"GF_AUTH_DISABLE_LOGIN_FORM=true",
			"GF_SECURITY_ADMIN_PASSWORD=admin",
		},
		ExposedPorts: nat.PortSet{
			"3000/tcp": struct{}{},
		},
		Labels: m.labels(),
	}

	hostConfig := &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:/etc/grafana/provisioning:ro", provisioningPath),
			fmt.Sprintf("%s:/var/lib/grafana/dashboards:ro", dashboardsPath),
		},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: volumeName,
				Target: "/var/lib/grafana",
			},
		},
		PortBindings: nat.PortMap{
			"3000/tcp": []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: fmt.Sprintf("%d", grafanaPort),
				},
			},
		},
		ExtraHosts: []string{
			"host.docker.internal:host-gateway",
		},
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
	}

	networkConfig := &network.NetworkingConfig{}

	resp, err := m.docker.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, containerName)
	if err != nil {
		return fmt.Errorf("failed to create Grafana container: %w", err)
	}

	// Start container
	if err := m.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start Grafana container: %w", err)
	}

	m.log.WithField("container_id", resp.ID[:12]).Debug("started Grafana container")

	return nil
}

// waitForHealth waits for observability services to be healthy.
func (m *ObservabilityManager) waitForHealth(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	ports := []int{
		m.servicePort(constants.ServicePrometheus),
		m.servicePort(constants.ServiceGrafana),
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for observability services")
		case <-ticker.C:
			allReady := true

			for _, port := range ports {
				addr := fmt.Sprintf("localhost:%d", port)

				conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
				if err != nil {
					allReady = false

					break
				}

				conn.Close()
			}

			if allReady {
				return nil
			}
		}
	}
}

// pullImageIfNeeded pulls a Docker image if not already present.
func (m *ObservabilityManager) pullImageIfNeeded(ctx context.Context, imageName string) error {
	// Check if image exists
	_, err := m.docker.ImageInspect(ctx, imageName)
	if err == nil {
		m.log.WithField("image", imageName).Debug("image already exists")

		return nil
	}

	m.log.WithField("image", imageName).Info("pulling Docker image")

	reader, err := m.docker.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}

	defer reader.Close()

	// Consume the output to complete the pull
	_, _ = io.Copy(io.Discard, reader)

	return nil
}

// isContainerRunning checks if a container is running.
func (m *ObservabilityManager) isContainerRunning(ctx context.Context, name string) (bool, error) {
	containerList, err := m.docker.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("name", name),
			filters.Arg("status", "running"),
		),
	})
	if err != nil {
		return false, err
	}

	for _, c := range containerList {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == name {
				if err := validateDockerLabels("container", name, c.Labels, m.labels()); err != nil {
					return false, err
				}

				return true, nil
			}
		}
	}

	return false, nil
}

// stopContainer stops and removes a container.
func (m *ObservabilityManager) stopContainer(ctx context.Context, name string) error {
	// Find the container
	containerList, err := m.docker.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("name", name),
		),
	})
	if err != nil {
		return err
	}

	for _, c := range containerList {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == name {
				// Stop container with timeout
				timeout := 10

				if err := m.docker.ContainerStop(ctx, c.ID, container.StopOptions{Timeout: &timeout}); err != nil {
					m.log.WithError(err).WithField("container", name).Debug("failed to stop container")
				}

				// Remove container
				if err := m.docker.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
					return fmt.Errorf("failed to remove container %s: %w", name, err)
				}

				m.log.WithField("container", name).Debug("stopped and removed container")

				return nil
			}
		}
	}

	return nil
}

// getContainerStatus gets the status of a container.
func (m *ObservabilityManager) getContainerStatus(ctx context.Context, service string) (ContainerStatus, error) {
	name := m.containerName(service)

	containerList, err := m.docker.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("name", name),
		),
	})
	if err != nil {
		return ContainerStatus{}, err
	}

	for _, c := range containerList {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == name {
				return ContainerStatus{
					Name:    service,
					State:   c.State,
					Running: c.State == "running",
					Port:    m.servicePort(service),
				}, nil
			}
		}
	}

	return ContainerStatus{}, fmt.Errorf("container %s not found", name)
}

func (m *ObservabilityManager) ensureVolume(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("volume name is required")
	}

	labels := m.labels()

	vol, err := m.docker.VolumeInspect(ctx, name)
	if err == nil {
		return validateDockerLabels("volume", name, vol.Labels, labels)
	}

	if !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("failed to inspect volume %s: %w", name, err)
	}

	_, err = m.docker.VolumeCreate(ctx, volume.CreateOptions{
		Name:   name,
		Labels: labels,
	})
	if err != nil {
		return fmt.Errorf("failed to create volume %s: %w", name, err)
	}

	return nil
}

func validateDockerLabels(resourceType, name string, actual, expected map[string]string) error {
	for key, expectedValue := range expected {
		if actual[key] != expectedValue {
			return fmt.Errorf(
				"%s %s exists without expected label %s=%s",
				resourceType,
				name,
				key,
				expectedValue,
			)
		}
	}

	return nil
}

// removeVolume removes a Docker volume.
func (m *ObservabilityManager) removeVolume(ctx context.Context, name string) error {
	if err := m.docker.VolumeRemove(ctx, name, true); err != nil {
		return fmt.Errorf("failed to remove volume %s: %w", name, err)
	}

	return nil
}
