// Package constants defines shared constants for service names,
// modes, and configuration across the xcli application.
package constants

import "fmt"

// Stack modes.
const (
	ModeLocal  = "local"
	ModeHybrid = "hybrid"
)

// Infrastructure modes.
const (
	InfraModeLocal    = "local"
	InfraModeExternal = "external"
)

// Service names and prefixes.
const (
	ServiceLabBackend   = "lab-backend"
	ServiceLabFrontend  = "lab-frontend"
	ServicePrefixCBT    = "cbt-"
	ServicePrefixCBTAPI = "cbt-api-"
)

// Binary names.
const (
	BinaryCBT        = "cbt"
	BinaryCBTAPI     = "server"
	BinaryLabBackend = "lab-backend"
)

// Directory names.
const (
	DirBin              = "bin"
	DirConfigs          = "configs"
	DirCustomConfigs    = "custom-configs"
	DirCustomDashboards = "custom-dashboards"
	DirLogs             = "logs"
	DirPIDs             = "pids"
)

// Config file templates.
const (
	ConfigFileCBT        = "cbt-%s.yaml"
	ConfigFileCBTAPI     = "cbt-api-%s.yaml"
	ConfigFileLabBackend = "lab-backend.yaml"
)

// Configuration files.
const (
	CBTOverridesFile = ".cbt-overrides.yaml"
)

// GitHub repository URLs.
const (
	GitHubOrg         = "ethpandaops"
	RepoCBT           = "cbt"
	RepoXatuCBT       = "xatu-cbt"
	RepoCBTAPI        = "cbt-api"
	RepoLabBackend    = "lab-backend"
	RepoLab           = "lab"
	GitHubURLTemplate = "https://github.com/%s/%s.git"
)

// Releasable projects.
const (
	ProjectCBT        = "cbt"
	ProjectXatuCBT    = "xatu-cbt"
	ProjectCBTAPI     = "cbt-api"
	ProjectLabBackend = "lab-backend"
	ProjectLab        = "lab"
)

// GitHub workflow files for dispatch triggers.
const (
	WorkflowXatuCBTDocker = "docker.yml"
)

// Observability stack.
const (
	// Service names.
	ServicePrometheus = "prometheus"
	ServiceGrafana    = "grafana"

	// Docker images.
	PrometheusImage = "prom/prometheus:v3.0.1"
	GrafanaImage    = "grafana/grafana:11.3.1"

	// Default ports.
	DefaultPrometheusPort = 9090
	DefaultGrafanaPort    = 3000

	// Container names.
	ContainerPrometheus = "xcli-prometheus"
	ContainerGrafana    = "xcli-grafana"

	// Volume names.
	VolumePrometheus = "xcli-prometheus-data"
	VolumeGrafana    = "xcli-grafana-data"
)

// PID file template.
const (
	PIDFileTemplate = "%s.pid"
)

// Log file template.
const (
	LogFileTemplate = "%s.log"
)

// Network genesis timestamps (Unix seconds).
var NetworkGenesisTimestamps = map[string]uint64{
	"mainnet": 1606824023, // Dec 1, 2020
	"sepolia": 1655733600, // Jun 20, 2022
	"hoodi":   1742213400, // Mar 15, 2025 (approximate)
	"holesky": 1695902400, // Sep 28, 2023 (legacy, use hoodi)
}

// ReleasableProjects lists all projects that can be released via xcli.
var ReleasableProjects = []string{
	ProjectCBT,
	ProjectCBTAPI,
	ProjectLab,
	ProjectLabBackend,
	ProjectXatuCBT,
}

// SemverProjects lists projects that use semver releases (tag-triggered).
var SemverProjects = []string{
	ProjectCBT,
	ProjectCBTAPI,
	ProjectLab,
	ProjectLabBackend,
}

// ProjectDependencies defines which projects depend on other projects.
// Key is the dependent project, value is slice of projects it depends on.
var ProjectDependencies = map[string][]string{
	ProjectXatuCBT:    {ProjectCBT}, // xatu-cbt imports cbt
	ProjectLabBackend: {ProjectLab}, // lab-backend bundles lab frontend
}

// ProjectRepoNames maps project names to their repository directory names.
// Used to look up local repo paths from LabReposConfig.
var ProjectRepoNames = map[string]string{
	ProjectCBT:        "cbt",
	ProjectXatuCBT:    "xatuCbt",
	ProjectCBTAPI:     "cbtApi",
	ProjectLabBackend: "labBackend",
}

// GetGitHubURL returns the GitHub clone URL for a repository.
func GetGitHubURL(repo string) string {
	return fmt.Sprintf(GitHubURLTemplate, GitHubOrg, repo)
}

// GetDependencies returns the projects that the given project depends on.
func GetDependencies(project string) []string {
	if deps, ok := ProjectDependencies[project]; ok {
		return deps
	}

	return nil
}

// GetDependents returns the projects that depend on the given project.
func GetDependents(project string) []string {
	dependents := make([]string, 0, len(ProjectDependencies))

	for dependent, deps := range ProjectDependencies {
		for _, dep := range deps {
			if dep == project {
				dependents = append(dependents, dependent)

				break
			}
		}
	}

	return dependents
}

// HasDependency returns true if 'dependent' depends on 'dependency'.
func HasDependency(dependent, dependency string) bool {
	deps := GetDependencies(dependent)
	for _, d := range deps {
		if d == dependency {
			return true
		}
	}

	return false
}

// GetRepoConfigKey returns the config key for a project's repo path.
func GetRepoConfigKey(project string) string {
	if key, ok := ProjectRepoNames[project]; ok {
		return key
	}

	return ""
}

// Service name helpers.
// ServiceName returns the full service name for a network-specific service.
func ServiceNameCBT(network string) string {
	return ServicePrefixCBT + network
}

// ServiceNameCBTAPI returns the full service name for a cbt-api instance.
func ServiceNameCBTAPI(network string) string {
	return ServicePrefixCBTAPI + network
}
