package cc

import (
	"context"
	"encoding/json"
)

// StackCapabilities describes which UI features a stack supports.
type StackCapabilities struct {
	HasEditableConfig bool `json:"hasEditableConfig"`
	HasServiceConfigs bool `json:"hasServiceConfigs"`
	HasCBTOverrides   bool `json:"hasCbtOverrides"`
	HasRedis          bool `json:"hasRedis"`
	HasGitRepos       bool `json:"hasGitRepos"`
	HasRegenerate     bool `json:"hasRegenerate"`
	HasRebuild        bool `json:"hasRebuild"`
}

// ProgressFunc reports stack lifecycle progress.
// phase is a short identifier, message is human-readable.
type ProgressFunc func(phase string, message string)

// LogSourceInfo describes how to stream logs for a service.
type LogSourceInfo struct {
	// Type is "file" or "docker".
	Type string
	// Path is the log file path (when Type == "file").
	Path string
	// Container is the Docker container name (when Type == "docker").
	Container string
}

// StackBackend abstracts stack-specific logic so the CC server and API
// handler can work with any stack type (Lab, Xatu, etc.) uniformly.
type StackBackend interface {
	// Name returns the stack's short identifier (e.g. "lab", "xatu").
	Name() string
	// Label returns the stack's display name (e.g. "Lab", "Xatu").
	Label() string
	// Capabilities returns which UI features this stack supports.
	Capabilities() StackCapabilities

	// Services & status
	GetServices(ctx context.Context) []serviceResponse
	GetConfigSummary() any

	// Lifecycle
	Up(ctx context.Context, progress ProgressFunc) error
	Down(ctx context.Context, progress ProgressFunc) error

	// Per-service actions
	StartService(ctx context.Context, name string) error
	StopService(ctx context.Context, name string) error
	RestartService(ctx context.Context, name string) error
	RebuildService(ctx context.Context, name string) error

	// Logs
	LogSource(name string) LogSourceInfo
	LogFilePath(name string) string

	// Git repos (empty map if N/A)
	GitRepos() map[string]string

	// Config management (return errors for unsupported operations)
	GetEditableConfig() (any, error)
	PutEditableConfig(data json.RawMessage) error
	GetOverrides() (any, error)
	PutOverrides(data json.RawMessage) error
	GetConfigFiles() ([]configFileInfo, error)
	GetConfigFile(name string) (*configFileContent, error)
	PutConfigFileOverride(name, content string) error
	DeleteConfigFileOverride(name string) error
	Regenerate(ctx context.Context) error

	// RecreateOrchestrator rebuilds internal orchestration after config changes.
	// No-op for stacks that don't use an orchestrator.
	RecreateOrchestrator() error

	// StateDir returns the state directory path (empty if N/A).
	StateDir() string
}
