package instance

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	xcligit "github.com/ethpandaops/xcli/pkg/git"
	"github.com/ethpandaops/xcli/pkg/workspace"
)

const (
	// SchemaVersion is the manifest schema version written by this xcli build.
	SchemaVersion = 1

	StatusCreated  = "created"
	StatusReserved = "reserved"
	StatusRunning  = "running"
	StatusStopped  = "stopped"
	StatusStale    = "stale"
)

// Manifest records discoverable state for one xcli lab instance.
type Manifest struct {
	SchemaVersion int                            `json:"schemaVersion"`
	InstanceID    string                         `json:"instanceId"`
	Status        string                         `json:"status"`
	RootDir       string                         `json:"rootDir"`
	ConfigPath    string                         `json:"configPath"`
	OverridesPath string                         `json:"overridesPath"`
	StateDir      string                         `json:"stateDir"`
	Mode          string                         `json:"mode"`
	Repos         map[string]xcligit.RepoVersion `json:"repos"`
	Ports         PortPlan                       `json:"ports"`
	Docker        DockerPlan                     `json:"docker"`
	PIDs          map[string]int                 `json:"pids"`
	URLs          map[string]string              `json:"urls"`
	CreatedAt     time.Time                      `json:"createdAt"`
	UpdatedAt     time.Time                      `json:"updatedAt"`
	LastError     string                         `json:"lastError,omitempty"`
}

// PortPlan is the complete set of host ports managed for an instance.
type PortPlan struct {
	Slot                 int                        `json:"slot"`
	LabBackend           int                        `json:"labBackend,omitempty"`
	LabFrontend          int                        `json:"labFrontend,omitempty"`
	CBTBase              int                        `json:"cbtBase,omitempty"`
	CBTAPIBase           int                        `json:"cbtApiBase,omitempty"`
	CBTFrontendBase      int                        `json:"cbtFrontendBase,omitempty"`
	CBTMetricsBase       int                        `json:"cbtMetricsBase,omitempty"`
	CBTAPIMetricsBase    int                        `json:"cbtApiMetricsBase,omitempty"`
	ClickHouseXatu01HTTP int                        `json:"clickhouseXatu01Http,omitempty"`
	ClickHouseXatu01TCP  int                        `json:"clickhouseXatu01Tcp,omitempty"`
	ClickHouseXatu02HTTP int                        `json:"clickhouseXatu02Http,omitempty"`
	ClickHouseXatu02TCP  int                        `json:"clickhouseXatu02Tcp,omitempty"`
	ClickHouseCBT01HTTP  int                        `json:"clickhouseCbt01Http,omitempty"`
	ClickHouseCBT01TCP   int                        `json:"clickhouseCbt01Tcp,omitempty"`
	ClickHouseCBT02HTTP  int                        `json:"clickhouseCbt02Http,omitempty"`
	ClickHouseCBT02TCP   int                        `json:"clickhouseCbt02Tcp,omitempty"`
	Redis                int                        `json:"redis,omitempty"`
	Prometheus           int                        `json:"prometheus,omitempty"`
	Grafana              int                        `json:"grafana,omitempty"`
	CommandCenter        int                        `json:"commandCenter,omitempty"`
	Networks             map[string]NetworkPortPlan `json:"networks,omitempty"`
}

// NetworkPortPlan records derived per-network application and metrics ports.
type NetworkPortPlan struct {
	CBT           int `json:"cbt,omitempty"`
	CBTAPI        int `json:"cbtApi,omitempty"`
	CBTFrontend   int `json:"cbtFrontend,omitempty"`
	CBTMetrics    int `json:"cbtMetrics,omitempty"`
	CBTAPIMetrics int `json:"cbtApiMetrics,omitempty"`
}

// DockerPlan records deterministic Docker names for one instance.
type DockerPlan struct {
	ProjectName string            `json:"projectName"`
	Containers  map[string]string `json:"containers"`
	Volumes     map[string]string `json:"volumes"`
	Labels      map[string]string `json:"labels"`
}

// NewManifest builds a manifest shell for the resolved lab workspace.
func NewManifest(
	ctx context.Context,
	ws *workspace.Workspace,
	labCfg *config.LabConfig,
	cliInstanceID string,
) (*Manifest, error) {
	if ws == nil {
		return nil, fmt.Errorf("workspace is required")
	}
	if labCfg == nil {
		return nil, fmt.Errorf("lab config is required")
	}

	instanceID, err := ResolveID(ws, labCfg, cliInstanceID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	repos, snapshotErr := SnapshotLabRepos(ctx, labCfg)
	lastError := ""
	if snapshotErr != nil {
		lastError = snapshotErr.Error()
	}

	return &Manifest{
		SchemaVersion: SchemaVersion,
		InstanceID:    instanceID,
		Status:        StatusCreated,
		RootDir:       ws.RootDir,
		ConfigPath:    ws.ConfigPath,
		OverridesPath: ws.OverridesPath,
		StateDir:      InstanceStateDir(ws.RootDir, instanceID),
		Mode:          labCfg.Mode,
		Repos:         repos,
		Ports:         PortPlan{},
		Docker:        NewDockerPlan(instanceID, ws.ConfigPath),
		PIDs:          map[string]int{},
		URLs:          map[string]string{},
		CreatedAt:     now,
		UpdatedAt:     now,
		LastError:     lastError,
	}, nil
}

// SnapshotLabRepos snapshots each configured lab repository without fetching.
func SnapshotLabRepos(ctx context.Context, labCfg *config.LabConfig) (map[string]xcligit.RepoVersion, error) {
	repoPaths := labCfg.Repos.Map()

	repos := make(map[string]xcligit.RepoVersion, len(repoPaths))
	errs := make([]string, 0)

	for name, path := range repoPaths {
		version, err := xcligit.Snapshot(ctx, path)
		if err != nil {
			version.Path = path
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
		}

		repos[name] = version
	}

	if len(errs) > 0 {
		return repos, errors.New(strings.Join(errs, "; "))
	}

	return repos, nil
}

// InstanceStateDir returns the per-instance state directory under a root.
func InstanceStateDir(rootDir, instanceID string) string {
	return filepath.Join(rootDir, ".xcli", "instances", instanceID)
}

// LocalManifestPath returns the root-local manifest copy path.
func LocalManifestPath(rootDir, instanceID string) string {
	return filepath.Join(InstanceStateDir(rootDir, instanceID), "manifest.json")
}

// NewDockerPlan returns deterministic Docker names for this instance.
func NewDockerPlan(instanceID, configPath string) DockerPlan {
	labels := map[string]string{
		DockerLabelInstance: instanceID,
		DockerLabelConfig:   configPath,
	}

	containers := map[string]string{
		constants.ServicePrometheus: fmt.Sprintf("xcli-%s-%s", instanceID, constants.ServicePrometheus),
		constants.ServiceGrafana:    fmt.Sprintf("xcli-%s-%s", instanceID, constants.ServiceGrafana),
	}

	volumes := map[string]string{
		constants.ServicePrometheus: fmt.Sprintf("xcli-%s-%s-data", instanceID, constants.ServicePrometheus),
		constants.ServiceGrafana:    fmt.Sprintf("xcli-%s-%s-data", instanceID, constants.ServiceGrafana),
	}

	return DockerPlan{
		ProjectName: fmt.Sprintf("xcli-%s", instanceID),
		Containers:  containers,
		Volumes:     volumes,
		Labels:      labels,
	}
}

// EffectiveDockerPlan returns the manifest Docker plan with deterministic
// fallbacks filled from the instance id and config path.
func (m *Manifest) EffectiveDockerPlan() DockerPlan {
	if m == nil {
		return DockerPlan{}
	}

	return m.Docker.WithDefaults(NewDockerPlan(m.InstanceID, m.ConfigPath))
}

// WithDefaults fills missing Docker plan fields from fallback.
func (p DockerPlan) WithDefaults(fallback DockerPlan) DockerPlan {
	if p.ProjectName == "" {
		p.ProjectName = fallback.ProjectName
	}
	if len(p.Containers) == 0 {
		p.Containers = fallback.Containers
	}
	if len(p.Volumes) == 0 {
		p.Volumes = fallback.Volumes
	}
	if len(p.Labels) == 0 {
		p.Labels = fallback.Labels
	}

	return p
}
