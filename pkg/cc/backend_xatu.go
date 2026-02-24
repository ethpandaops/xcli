package cc

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/ethpandaops/xcli/pkg/compose"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/git"
	"github.com/sirupsen/logrus"
)

// xatuConfigResponse is the editable config for the Xatu stack.
type xatuConfigResponse struct {
	Profiles     []string          `json:"profiles"`
	EnvOverrides map[string]string `json:"envOverrides"`
	RepoPath     string            `json:"repoPath"`
}

// xatuConfigRequest is the request body for updating Xatu config.
type xatuConfigRequest struct {
	Profiles     []string          `json:"profiles"`
	EnvOverrides map[string]string `json:"envOverrides"`
}

// xatuBackend implements StackBackend for the Xatu docker-compose stack.
type xatuBackend struct {
	log     logrus.FieldLogger
	runner  *compose.Runner
	xatuCfg *config.XatuConfig
	cfgPath string
	gitChk  *git.Checker
}

// Compile-time interface check.
var _ StackBackend = (*xatuBackend)(nil)

// newXatuBackend creates a new Xatu backend.
func newXatuBackend(
	log logrus.FieldLogger,
	xatuCfg *config.XatuConfig,
	cfgPath string,
	gitChk *git.Checker,
) (*xatuBackend, error) {
	runner, err := compose.NewRunner(
		log, xatuCfg.Repos.Xatu,
		xatuCfg.Profiles, xatuCfg.EnvOverrides,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create compose runner: %w", err)
	}

	return &xatuBackend{
		log:     log,
		runner:  runner,
		xatuCfg: xatuCfg,
		cfgPath: cfgPath,
		gitChk:  gitChk,
	}, nil
}

// Name returns "xatu".
func (b *xatuBackend) Name() string { return "xatu" }

// Label returns "Xatu".
func (b *xatuBackend) Label() string { return "Xatu" }

// Capabilities returns which UI features Xatu supports.
func (b *xatuBackend) Capabilities() StackCapabilities {
	return StackCapabilities{
		HasEditableConfig: true,
		HasServiceConfigs: false,
		HasCBTOverrides:   false,
		HasRedis:          false,
		HasGitRepos:       true,
		HasRegenerate:     false,
		HasRebuild:        true,
	}
}

// GetServices returns service statuses by merging the compose config
// (which lists all defined services) with runtime status from docker compose ps.
// This ensures services are visible even when the stack is stopped.
func (b *xatuBackend) GetServices(ctx context.Context) []serviceResponse {
	// Get all service names from the compose config (always available).
	serviceNames, err := b.runner.ListServices(ctx)
	if err != nil {
		b.log.WithError(err).Warn("Failed to list compose services")

		return []serviceResponse{}
	}

	// Sort for stable ordering across refreshes.
	slices.Sort(serviceNames)

	// Build a lookup of running service statuses.
	statusMap := make(map[string]compose.ServiceStatus, len(serviceNames))

	statuses, err := b.runner.PS(ctx)
	if err != nil {
		b.log.WithError(err).Debug("Failed to get docker compose ps (stack may be down)")
	}

	for _, svc := range statuses {
		statusMap[svc.Service] = svc
	}

	// Merge: use config as the base, overlay runtime status.
	result := make([]serviceResponse, 0, len(serviceNames))

	for _, name := range serviceNames {
		status := stackStatusStopped
		health := "unknown"

		if svc, ok := statusMap[name]; ok {
			switch svc.State {
			case stackStatusRunning:
				status = stackStatusRunning
				health = "healthy"
			case "exited":
				status = "exited"
			}
		}

		result = append(result, serviceResponse{
			Name:   name,
			Status: status,
			Health: health,
		})
	}

	return result
}

// GetConfigSummary returns a summary for the sidebar.
func (b *xatuBackend) GetConfigSummary() any {
	return xatuConfigResponse{
		Profiles:     b.xatuCfg.Profiles,
		EnvOverrides: b.xatuCfg.EnvOverrides,
		RepoPath:     b.xatuCfg.Repos.Xatu,
	}
}

// Up starts the docker-compose stack with granular progress phases.
func (b *xatuBackend) Up(ctx context.Context, progress ProgressFunc) error {
	// Phase 1: Validate configuration.
	if progress != nil {
		progress("validate_config", "Validating xatu configuration...")
	}

	if err := b.xatuCfg.Validate(); err != nil {
		return fmt.Errorf("invalid xatu configuration: %w", err)
	}

	// Phase 2: Pull latest images.
	if progress != nil {
		progress("pull_images", "Pulling docker images...")
	}

	if err := b.runner.Pull(ctx); err != nil {
		b.log.WithError(err).Warn("Failed to pull images, continuing with local images")
	}

	// Phase 3: Start containers.
	if progress != nil {
		progress("compose_up", "Starting containers...")
	}

	return b.runner.Up(ctx, false)
}

// Down stops the docker-compose stack.
func (b *xatuBackend) Down(ctx context.Context, progress ProgressFunc) error {
	if progress != nil {
		progress("compose_down", "Stopping docker compose stack...")
	}

	return b.runner.Down(ctx, false, false)
}

// StartService starts a single docker compose service.
func (b *xatuBackend) StartService(ctx context.Context, name string) error {
	return b.runner.Start(ctx, name)
}

// StopService stops a single docker compose service.
func (b *xatuBackend) StopService(ctx context.Context, name string) error {
	return b.runner.Stop(ctx, name)
}

// RestartService restarts a single docker compose service.
func (b *xatuBackend) RestartService(ctx context.Context, name string) error {
	return b.runner.Restart(ctx, name)
}

// RebuildService rebuilds and restarts a single docker compose service.
func (b *xatuBackend) RebuildService(ctx context.Context, name string) error {
	if err := b.runner.Build(ctx, name); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	return b.runner.Up(ctx, false, name)
}

// LogSource returns docker container log source info for a service.
func (b *xatuBackend) LogSource(name string) LogSourceInfo {
	// For docker compose services, use the service name as the container
	// identifier for docker log streaming.
	statuses, err := b.runner.PS(context.Background())
	if err == nil {
		for _, svc := range statuses {
			if svc.Service == name && svc.Name != "" {
				return LogSourceInfo{Type: "docker", Container: svc.Name}
			}
		}
	}

	return LogSourceInfo{Type: "docker", Container: name}
}

// LogFilePath returns empty — Xatu uses docker logs, not file-based logs.
func (b *xatuBackend) LogFilePath(_ string) string {
	return ""
}

// GitRepos returns the Xatu repository for git status.
func (b *xatuBackend) GitRepos() map[string]string {
	return map[string]string{
		"xatu": b.xatuCfg.Repos.Xatu,
	}
}

// GetEditableConfig returns the Xatu config (profiles, env overrides).
func (b *xatuBackend) GetEditableConfig() (any, error) {
	return xatuConfigResponse{
		Profiles:     b.xatuCfg.Profiles,
		EnvOverrides: b.xatuCfg.EnvOverrides,
		RepoPath:     b.xatuCfg.Repos.Xatu,
	}, nil
}

// PutEditableConfig updates Xatu profiles and env overrides.
func (b *xatuBackend) PutEditableConfig(data json.RawMessage) error {
	var req xatuConfigRequest

	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}

	b.xatuCfg.Profiles = req.Profiles
	b.xatuCfg.EnvOverrides = req.EnvOverrides

	fullCfg := &config.Config{Xatu: b.xatuCfg}

	if err := fullCfg.Save(b.cfgPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Recreate runner with new profiles/env
	runner, err := compose.NewRunner(
		b.log, b.xatuCfg.Repos.Xatu,
		b.xatuCfg.Profiles, b.xatuCfg.EnvOverrides,
	)
	if err != nil {
		return fmt.Errorf("failed to recreate compose runner: %w", err)
	}

	b.runner = runner

	return nil
}

// GetOverrides returns an error — Xatu has no CBT overrides.
func (b *xatuBackend) GetOverrides() (any, error) {
	return nil, fmt.Errorf("overrides not supported for xatu stack")
}

// PutOverrides returns an error — Xatu has no CBT overrides.
func (b *xatuBackend) PutOverrides(_ json.RawMessage) error {
	return fmt.Errorf("overrides not supported for xatu stack")
}

// GetConfigFiles returns an error — Xatu has no generated config files.
func (b *xatuBackend) GetConfigFiles() ([]configFileInfo, error) {
	return nil, fmt.Errorf("config files not supported for xatu stack")
}

// GetConfigFile returns an error — Xatu has no generated config files.
func (b *xatuBackend) GetConfigFile(_ string) (*configFileContent, error) {
	return nil, fmt.Errorf("config files not supported for xatu stack")
}

// PutConfigFileOverride returns an error — Xatu has no config file overrides.
func (b *xatuBackend) PutConfigFileOverride(_, _ string) error {
	return fmt.Errorf("config file overrides not supported for xatu stack")
}

// DeleteConfigFileOverride returns an error — Xatu has no config file overrides.
func (b *xatuBackend) DeleteConfigFileOverride(_ string) error {
	return fmt.Errorf("config file overrides not supported for xatu stack")
}

// Regenerate returns an error — Xatu has no config regeneration.
func (b *xatuBackend) Regenerate(_ context.Context) error {
	return fmt.Errorf("regeneration not supported for xatu stack")
}

// RecreateOrchestrator is a no-op for Xatu — no orchestrator.
func (b *xatuBackend) RecreateOrchestrator() error {
	return nil
}

// StateDir returns empty — Xatu has no state directory.
func (b *xatuBackend) StateDir() string {
	return ""
}
