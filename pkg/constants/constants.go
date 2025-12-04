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
	DirBin           = "bin"
	DirConfigs       = "configs"
	DirCustomConfigs = "custom-configs"
	DirLogs          = "logs"
	DirPIDs          = "pids"
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

// GetGitHubURL returns the GitHub clone URL for a repository.
func GetGitHubURL(repo string) string {
	return fmt.Sprintf(GitHubURLTemplate, GitHubOrg, repo)
}

// Network genesis timestamps (Unix seconds).
var NetworkGenesisTimestamps = map[string]uint64{
	"mainnet": 1606824023, // Dec 1, 2020
	"sepolia": 1655733600, // Jun 20, 2022
	"hoodi":   1742213400, // Mar 15, 2025 (approximate)
	"holesky": 1695902400, // Sep 28, 2023 (legacy, use hoodi)
}

// Releasable projects.
const (
	ProjectCBT        = "cbt"
	ProjectXatuCBT    = "xatu-cbt"
	ProjectCBTAPI     = "cbt-api"
	ProjectLabBackend = "lab-backend"
)

// GitHub workflow files for dispatch triggers.
const (
	WorkflowXatuCBTDocker = "docker.yml"
)

// ReleasableProjects lists all projects that can be released via xcli.
var ReleasableProjects = []string{
	ProjectCBT,
	ProjectCBTAPI,
	ProjectLabBackend,
	ProjectXatuCBT,
}

// SemverProjects lists projects that use semver releases (tag-triggered).
var SemverProjects = []string{
	ProjectCBT,
	ProjectCBTAPI,
	ProjectLabBackend,
}

// PID file template.
const (
	PIDFileTemplate = "%s.pid"
)

// Log file template.
const (
	LogFileTemplate = "%s.log"
)

// Service name helpers.
// ServiceName returns the full service name for a network-specific service.
func ServiceNameCBT(network string) string {
	return ServicePrefixCBT + network
}

// ServiceNameCBTAPI returns the full service name for a cbt-api instance.
func ServiceNameCBTAPI(network string) string {
	return ServicePrefixCBTAPI + network
}
