package instance

import (
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

const (
	StatusOrphan = "orphan"

	// DockerResourceKindContainer is the DockerResource.Kind value for containers.
	DockerResourceKindContainer = "container"

	DockerLabelInstance = "com.ethpandaops.xcli.instance"
	DockerLabelConfig   = "com.ethpandaops.xcli.config"

	DockerComposeProjectLabel = "com.docker.compose.project"
)

// DockerResource is a labeled Docker resource discovered during reconciliation.
type DockerResource struct {
	Kind   string
	ID     string
	Name   string
	State  string
	Labels map[string]string
}

// DockerResourceProvider lists Docker resources labeled with an xcli instance id.
type DockerResourceProvider interface {
	ListXCLIResources(ctx context.Context) ([]DockerResource, error)
}

// Reconciler compares persisted manifests with live host state.
type Reconciler struct {
	Registry    *Registry
	Docker      DockerResourceProvider
	PIDAlive    func(pid int) bool
	PortBound   func(port int) bool
	DockerError error
}

// ReconcileResult is a snapshot of all known and discovered instances.
type ReconcileResult struct {
	Instances   []*ReconciledInstance
	DockerError error
}

// ReconciledInstance is one manifest plus live-state observations.
type ReconciledInstance struct {
	Manifest        *Manifest
	InstanceID      string
	Status          string
	RegistryPresent bool
	Orphan          bool
	Live            LiveState
}

// LiveState records observed backing resources for an instance.
type LiveState struct {
	PIDs            map[string]bool
	Ports           map[string]bool
	DockerResources []DockerResource
	BackingExists   bool
}

// NewReconciler creates a reconciler backed by the default host checks.
func NewReconciler(registry *Registry) *Reconciler {
	return &Reconciler{Registry: registry}
}

// ReconcileAll reads the registry and reconciles it against live resources.
func (r *Reconciler) ReconcileAll(ctx context.Context) (*ReconcileResult, error) {
	registry := r.Registry

	var err error
	if registry == nil {
		registry, err = DefaultRegistry()
		if err != nil {
			return nil, err
		}
	}

	manifests, err := registry.LoadAll()
	if err != nil {
		return nil, err
	}

	resources, dockerErr := r.listDockerResources(ctx)
	resourcesByInstance := make(map[string][]DockerResource)

	for _, resource := range resources {
		instanceID := dockerResourceInstanceID(resource)
		if instanceID == "" {
			continue
		}

		resourcesByInstance[instanceID] = append(resourcesByInstance[instanceID], resource)
	}

	seen := make(map[string]bool, len(manifests))
	instances := make([]*ReconciledInstance, 0, len(manifests)+len(resourcesByInstance))

	for _, manifest := range manifests {
		if manifest == nil || manifest.InstanceID == "" {
			continue
		}

		seen[manifest.InstanceID] = true
		instances = append(instances, r.reconcileManifest(manifest, true, resourcesByInstance[manifest.InstanceID]))
	}

	orphanIDs := make([]string, 0)

	for instanceID := range resourcesByInstance {
		if !seen[instanceID] {
			orphanIDs = append(orphanIDs, instanceID)
		}
	}

	sort.Strings(orphanIDs)

	for _, instanceID := range orphanIDs {
		instances = append(instances, r.reconcileOrphan(instanceID, resourcesByInstance[instanceID]))
	}

	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceID < instances[j].InstanceID
	})

	return &ReconcileResult{Instances: instances, DockerError: dockerErr}, nil
}

func (r *Reconciler) reconcileManifest(
	manifest *Manifest,
	registryPresent bool,
	resources []DockerResource,
) *ReconciledInstance {
	live := LiveState{
		PIDs:            r.checkPIDs(manifest.PIDs),
		Ports:           r.checkPorts(manifest.Ports),
		DockerResources: resources,
	}
	live.BackingExists = anyTrue(live.PIDs) || anyTrue(live.Ports) || hasDockerContainer(resources)

	status := manifest.Status
	if status == "" {
		status = StatusCreated
	}

	if registryPresent &&
		(manifest.Status == StatusRunning || manifest.Status == StatusReserved) &&
		!live.BackingExists {
		status = StatusStale
	}

	return &ReconciledInstance{
		Manifest:        manifest,
		InstanceID:      manifest.InstanceID,
		Status:          status,
		RegistryPresent: registryPresent,
		Live:            live,
	}
}

func (r *Reconciler) reconcileOrphan(instanceID string, resources []DockerResource) *ReconciledInstance {
	configPath := ""
	for _, resource := range resources {
		if configPath = strings.TrimSpace(resource.Labels[DockerLabelConfig]); configPath != "" {
			break
		}
	}

	manifest := &Manifest{
		SchemaVersion: SchemaVersion,
		InstanceID:    instanceID,
		Status:        StatusOrphan,
		ConfigPath:    configPath,
		PIDs:          map[string]int{},
		URLs:          map[string]string{},
	}
	manifest.Docker = manifest.EffectiveDockerPlan()

	return &ReconciledInstance{
		Manifest:        manifest,
		InstanceID:      instanceID,
		Status:          StatusOrphan,
		RegistryPresent: false,
		Orphan:          true,
		Live: LiveState{
			PIDs:            map[string]bool{},
			Ports:           map[string]bool{},
			DockerResources: resources,
			BackingExists:   len(resources) > 0,
		},
	}
}

func (r *Reconciler) listDockerResources(ctx context.Context) ([]DockerResource, error) {
	provider := r.Docker
	if provider == nil {
		var err error

		provider, err = NewDockerResourceProvider()
		if err != nil {
			return nil, err
		}
	}

	return provider.ListXCLIResources(ctx)
}

func (r *Reconciler) checkPIDs(pids map[string]int) map[string]bool {
	result := make(map[string]bool, len(pids))

	check := r.PIDAlive
	if check == nil {
		check = defaultPIDAlive
	}

	for name, pid := range pids {
		result[name] = pid > 0 && check(pid)
	}

	return result
}

func (r *Reconciler) checkPorts(plan PortPlan) map[string]bool {
	ports := plan.NamedPorts()
	result := make(map[string]bool, len(ports))

	check := r.PortBound
	if check == nil {
		check = defaultPortBound
	}

	names := make([]string, 0, len(ports))
	for name := range ports {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		port := ports[name]
		if port > 0 {
			result[name] = check(port)
		}
	}

	return result
}

func anyTrue(values map[string]bool) bool {
	for _, value := range values {
		if value {
			return true
		}
	}

	return false
}

func dockerResourceInstanceID(resource DockerResource) string {
	instanceID := strings.TrimSpace(resource.Labels[DockerLabelInstance])
	if instanceID != "" {
		return instanceID
	}

	projectName := strings.TrimSpace(resource.Labels[DockerComposeProjectLabel])
	if strings.HasPrefix(projectName, "xcli-") && len(projectName) > len("xcli-") {
		return strings.TrimPrefix(projectName, "xcli-")
	}

	return ""
}

func hasDockerContainer(resources []DockerResource) bool {
	for _, resource := range resources {
		if resource.Kind == DockerResourceKindContainer {
			return true
		}
	}

	return false
}

func defaultPIDAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	return proc.Signal(syscall.Signal(0)) == nil
}

func defaultPortBound(port int) bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 250*time.Millisecond)
	if err != nil {
		return false
	}

	_ = conn.Close()

	return true
}

type dockerResourceProvider struct {
	client *client.Client
}

// NewDockerResourceProvider returns a Docker-backed resource provider.
func NewDockerResourceProvider() (DockerResourceProvider, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &dockerResourceProvider{client: dockerClient}, nil
}

func (p *dockerResourceProvider) ListXCLIResources(ctx context.Context) ([]DockerResource, error) {
	resources := make([]DockerResource, 0)
	seen := make(map[string]bool)

	for _, label := range []string{DockerLabelInstance, DockerComposeProjectLabel} {
		filterArgs := filters.NewArgs(filters.Arg("label", label))

		containers, err := p.client.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
		if err != nil {
			return nil, fmt.Errorf("failed to list Docker containers: %w", err)
		}

		for _, item := range containers {
			name := item.ID
			if len(item.Names) > 0 {
				name = strings.TrimPrefix(item.Names[0], "/")
			}

			key := "container:" + item.ID
			if seen[key] {
				continue
			}

			seen[key] = true

			resources = append(resources, DockerResource{
				Kind:   DockerResourceKindContainer,
				ID:     item.ID,
				Name:   name,
				State:  item.State,
				Labels: item.Labels,
			})
		}

		volumes, err := p.client.VolumeList(ctx, volume.ListOptions{Filters: filterArgs})
		if err != nil {
			return nil, fmt.Errorf("failed to list Docker volumes: %w", err)
		}

		for _, item := range volumes.Volumes {
			key := "volume:" + item.Name
			if seen[key] {
				continue
			}

			seen[key] = true

			resources = append(resources, DockerResource{
				Kind:   "volume",
				Name:   item.Name,
				State:  "present",
				Labels: item.Labels,
			})
		}
	}

	return resources, nil
}
