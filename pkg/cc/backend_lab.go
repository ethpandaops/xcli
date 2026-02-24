package cc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/configtui"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/git"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/seeddata"
	"github.com/ethpandaops/xcli/pkg/tui"
	"github.com/sirupsen/logrus"
)

// labBackend implements StackBackend for the Lab orchestrator-based stack.
type labBackend struct {
	log     logrus.FieldLogger
	wrapper *tui.OrchestratorWrapper
	orch    *orchestrator.Orchestrator
	labCfg  *config.LabConfig
	cfgPath string
	gitChk  *git.Checker
}

// Compile-time interface check.
var _ StackBackend = (*labBackend)(nil)

// newLabBackend creates a new Lab backend.
func newLabBackend(
	log logrus.FieldLogger,
	orch *orchestrator.Orchestrator,
	labCfg *config.LabConfig,
	cfgPath string,
	gitChk *git.Checker,
) *labBackend {
	return &labBackend{
		log:     log,
		wrapper: tui.NewOrchestratorWrapper(orch),
		orch:    orch,
		labCfg:  labCfg,
		cfgPath: cfgPath,
		gitChk:  gitChk,
	}
}

// Name returns "lab".
func (b *labBackend) Name() string { return "lab" }

// Label returns "Lab".
func (b *labBackend) Label() string { return "Lab" }

// Capabilities returns all features enabled for Lab.
func (b *labBackend) Capabilities() StackCapabilities {
	return StackCapabilities{
		HasEditableConfig: true,
		HasServiceConfigs: true,
		HasCBTOverrides:   true,
		HasRedis:          true,
		HasGitRepos:       true,
		HasRegenerate:     true,
		HasRebuild:        true,
	}
}

// GetServices returns current service and infrastructure statuses from the orchestrator,
// merged into a single unified list.
func (b *labBackend) GetServices(_ context.Context) []serviceResponse {
	services := b.wrapper.GetServices()
	infra := b.wrapper.GetInfrastructure()
	result := make([]serviceResponse, 0, len(services)+len(infra))

	for _, svc := range services {
		resp := serviceResponse{
			Name:    svc.Name,
			Status:  svc.Status,
			PID:     svc.PID,
			URL:     svc.URL,
			Ports:   svc.Ports,
			Health:  svc.Health,
			LogFile: svc.LogFile,
		}

		if svc.Uptime > 0 {
			resp.Uptime = formatDuration(svc.Uptime)
		}

		result = append(result, resp)
	}

	for _, i := range infra {
		result = append(result, serviceResponse{
			Name:   i.Name,
			Status: i.Status,
		})
	}

	return result
}

// GetConfigSummary returns the sanitized config for the sidebar.
func (b *labBackend) GetConfigSummary() any {
	networks := make([]networkInfo, 0, len(b.labCfg.Networks))
	for _, n := range b.labCfg.Networks {
		networks = append(networks, networkInfo{
			Name:       n.Name,
			Enabled:    n.Enabled,
			PortOffset: n.PortOffset,
		})
	}

	return configResponse{
		Mode:     b.labCfg.Mode,
		Networks: networks,
		Ports: portsInfo{
			LabBackend:      b.labCfg.Ports.LabBackend,
			LabFrontend:     b.labCfg.Ports.LabFrontend,
			CBTBase:         b.labCfg.Ports.CBTBase,
			CBTAPIBase:      b.labCfg.Ports.CBTAPIBase,
			CBTFrontendBase: b.labCfg.Ports.CBTFrontendBase,
			ClickHouseCBT:   b.labCfg.Infrastructure.ClickHouseCBTPort,
			ClickHouseXatu:  b.labCfg.Infrastructure.ClickHouseXatuPort,
			Redis:           b.labCfg.Infrastructure.RedisPort,
			Prometheus:      b.labCfg.Infrastructure.Observability.PrometheusPort,
			Grafana:         b.labCfg.Infrastructure.Observability.GrafanaPort,
		},
		CfgPath: b.cfgPath,
	}
}

// Up boots the full lab stack.
func (b *labBackend) Up(ctx context.Context, progress ProgressFunc) error {
	orchProgress := orchestrator.ProgressFunc(func(phase, message string) {
		if progress != nil {
			progress(phase, message)
		}
	})

	return b.orch.Up(ctx, false, false, orchProgress)
}

// Down tears down the full lab stack.
func (b *labBackend) Down(ctx context.Context, progress ProgressFunc) error {
	orchProgress := orchestrator.ProgressFunc(func(phase, message string) {
		if progress != nil {
			progress(phase, message)
		}
	})

	return b.orch.Down(ctx, orchProgress)
}

// StartService starts a single service.
func (b *labBackend) StartService(ctx context.Context, name string) error {
	return b.wrapper.StartService(ctx, name)
}

// StopService stops a single service.
func (b *labBackend) StopService(ctx context.Context, name string) error {
	return b.wrapper.StopService(ctx, name)
}

// RestartService restarts a single service.
func (b *labBackend) RestartService(ctx context.Context, name string) error {
	return b.wrapper.RestartService(ctx, name)
}

// RebuildService rebuilds and restarts a single service.
func (b *labBackend) RebuildService(ctx context.Context, name string) error {
	return b.wrapper.RebuildService(ctx, name)
}

// LogSource returns how to stream logs for a given service.
func (b *labBackend) LogSource(name string) LogSourceInfo {
	if container, ok := dockerContainerNames[name]; ok {
		return LogSourceInfo{Type: "docker", Container: container}
	}

	return LogSourceInfo{Type: "file", Path: b.orch.LogFilePath(name)}
}

// LogFilePath returns the log file path for a service.
func (b *labBackend) LogFilePath(name string) string {
	return b.orch.LogFilePath(name)
}

// GitRepos returns the lab repositories for git status checking.
func (b *labBackend) GitRepos() map[string]string {
	return map[string]string{
		"cbt":         b.labCfg.Repos.CBT,
		"xatu-cbt":    b.labCfg.Repos.XatuCBT,
		"cbt-api":     b.labCfg.Repos.CBTAPI,
		"lab-backend": b.labCfg.Repos.LabBackend,
		"lab":         b.labCfg.Repos.Lab,
	}
}

// GetEditableConfig returns the lab config with passwords masked.
func (b *labBackend) GetEditableConfig() (any, error) {
	resp := labConfigResponse{
		Mode:           b.labCfg.Mode,
		Networks:       b.labCfg.Networks,
		Infrastructure: b.labCfg.Infrastructure,
		Ports:          b.labCfg.Ports,
		Dev:            b.labCfg.Dev,
		Repos:          b.labCfg.Repos,
	}

	if resp.Infrastructure.ClickHouse.Xatu.ExternalPassword != "" {
		resp.Infrastructure.ClickHouse.Xatu.ExternalPassword = maskedPassword
	}

	if resp.Infrastructure.ClickHouse.CBT.ExternalPassword != "" {
		resp.Infrastructure.ClickHouse.CBT.ExternalPassword = maskedPassword
	}

	return resp, nil
}

// PutEditableConfig updates the lab config, validates, saves, and regenerates.
func (b *labBackend) PutEditableConfig(data json.RawMessage) error {
	var req labConfigResponse

	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}

	// Preserve masked passwords.
	if req.Infrastructure.ClickHouse.Xatu.ExternalPassword == maskedPassword {
		req.Infrastructure.ClickHouse.Xatu.ExternalPassword =
			b.labCfg.Infrastructure.ClickHouse.Xatu.ExternalPassword
	}

	if req.Infrastructure.ClickHouse.CBT.ExternalPassword == maskedPassword {
		req.Infrastructure.ClickHouse.CBT.ExternalPassword =
			b.labCfg.Infrastructure.ClickHouse.CBT.ExternalPassword
	}

	updated := &config.LabConfig{
		Mode:           req.Mode,
		Networks:       req.Networks,
		Infrastructure: req.Infrastructure,
		Ports:          req.Ports,
		Dev:            req.Dev,
		Repos:          req.Repos,
		TUI:            b.labCfg.TUI,
	}

	if err := updated.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	*b.labCfg = *updated

	fullCfg := &config.Config{Lab: b.labCfg}

	if err := fullCfg.Save(b.cfgPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if err := b.RecreateOrchestrator(); err != nil {
		return fmt.Errorf("failed to apply config: %w", err)
	}

	if err := b.orch.GenerateConfigs(context.Background()); err != nil {
		b.log.WithError(err).Error("Failed to regenerate configs after config update")
	}

	return nil
}

// GetOverrides returns the CBT overrides state.
func (b *labBackend) GetOverrides() (any, error) {
	xatuCBTPath := b.labCfg.Repos.XatuCBT
	stateDir := b.orch.StateDir()

	overridesPath := filepath.Join(
		filepath.Dir(stateDir), constants.CBTOverridesFile,
	)

	externalNames, transformNames, err := configtui.DiscoverModels(xatuCBTPath)
	if err != nil {
		return nil, fmt.Errorf("failed to discover models: %w", err)
	}

	overrides, fileExists, err := configtui.LoadOverrides(overridesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load overrides: %w", err)
	}

	deps := configtui.LoadDependencies(xatuCBTPath, transformNames)

	resp := cbtOverridesResponse{
		DefaultEnabled: overrides.Models.DefaultEnabled,
		ExternalModels: make(
			[]modelEntryResponse, 0, len(externalNames),
		),
		TransformationModels: make(
			[]modelEntryResponse, 0, len(transformNames),
		),
		Dependencies:        deps,
		EnvMinTimestamp:     overrides.Models.Env["EXTERNAL_MODEL_MIN_TIMESTAMP"],
		EnvTimestampEnabled: overrides.Models.Env["EXTERNAL_MODEL_MIN_TIMESTAMP"] != "",
		EnvMinBlock:         overrides.Models.Env["EXTERNAL_MODEL_MIN_BLOCK"],
		EnvBlockEnabled:     overrides.Models.Env["EXTERNAL_MODEL_MIN_BLOCK"] != "",
	}

	for _, name := range externalNames {
		overrideKey := name
		if db := seeddata.GetExternalModelDatabase(name, xatuCBTPath); db != "" {
			overrideKey = db + "." + name
		}

		enabled := fileExists && !configtui.IsModelDisabled(overrides, overrideKey)

		resp.ExternalModels = append(
			resp.ExternalModels, modelEntryResponse{
				Name:        name,
				OverrideKey: overrideKey,
				Enabled:     enabled,
			},
		)
	}

	for _, name := range transformNames {
		enabled := fileExists && !configtui.IsModelDisabled(overrides, name)

		resp.TransformationModels = append(
			resp.TransformationModels,
			modelEntryResponse{
				Name:        name,
				OverrideKey: name,
				Enabled:     enabled,
			},
		)
	}

	return resp, nil
}

// PutOverrides saves CBT overrides.
func (b *labBackend) PutOverrides(data json.RawMessage) error {
	var req cbtOverridesRequest

	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}

	stateDir := b.orch.StateDir()
	overridesPath := filepath.Join(
		filepath.Dir(stateDir), constants.CBTOverridesFile,
	)

	existingOverrides, _, err := configtui.LoadOverrides(overridesPath)
	if err != nil {
		return fmt.Errorf("failed to load existing overrides: %w", err)
	}

	externalModels := make(
		[]configtui.ModelEntry, 0, len(req.ExternalModels),
	)

	for _, m := range req.ExternalModels {
		overrideKey := m.OverrideKey
		if overrideKey == "" {
			overrideKey = m.Name
		}

		externalModels = append(externalModels, configtui.ModelEntry{
			Name:        m.Name,
			OverrideKey: overrideKey,
			Enabled:     m.Enabled,
		})
	}

	transformModels := make(
		[]configtui.ModelEntry, 0, len(req.TransformationModels),
	)

	for _, m := range req.TransformationModels {
		overrideKey := m.OverrideKey
		if overrideKey == "" {
			overrideKey = m.Name
		}

		transformModels = append(transformModels, configtui.ModelEntry{
			Name:        m.Name,
			OverrideKey: overrideKey,
			Enabled:     m.Enabled,
		})
	}

	if err := configtui.SaveOverridesFromEntries(
		overridesPath,
		externalModels,
		transformModels,
		req.EnvMinTimestamp,
		req.EnvTimestampEnabled,
		req.EnvMinBlock,
		req.EnvBlockEnabled,
		existingOverrides,
		req.DefaultEnabled,
	); err != nil {
		return fmt.Errorf("failed to save overrides: %w", err)
	}

	if err := b.orch.GenerateConfigs(context.Background()); err != nil {
		b.log.WithError(err).Error("Failed to regenerate configs after overrides save")
	}

	return nil
}

// GetConfigFiles lists generated config files with override status.
func (b *labBackend) GetConfigFiles() ([]configFileInfo, error) {
	configsDir := filepath.Join(b.orch.StateDir(), "configs")

	entries, err := os.ReadDir(configsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read configs directory: %w", err)
	}

	customDir := filepath.Join(b.orch.StateDir(), "custom-configs")
	files := make([]configFileInfo, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}

		_, hasOverride := os.Stat(
			filepath.Join(customDir, entry.Name()),
		)

		files = append(files, configFileInfo{
			Name:        entry.Name(),
			HasOverride: hasOverride == nil,
			Size:        info.Size(),
		})
	}

	return files, nil
}

// GetConfigFile returns the content of a generated config file.
func (b *labBackend) GetConfigFile(name string) (*configFileContent, error) {
	cleanName, ok := sanitizeConfigFileName(name)
	if !ok {
		return nil, fmt.Errorf("invalid file name")
	}

	configsDir := filepath.Join(b.orch.StateDir(), "configs")
	safePath := filepath.Join(configsDir, cleanName)

	content, err := os.ReadFile(safePath)
	if err != nil {
		return nil, fmt.Errorf("config file not found: %w", err)
	}

	resp := &configFileContent{
		Name:    cleanName,
		Content: string(content),
	}

	customPath := filepath.Join(
		b.orch.StateDir(), "custom-configs", cleanName,
	)

	overrideContent, overrideErr := os.ReadFile(customPath)
	if overrideErr == nil {
		resp.HasOverride = true
		resp.OverrideContent = string(overrideContent)
	}

	return resp, nil
}

// PutConfigFileOverride saves a custom override for a config file.
func (b *labBackend) PutConfigFileOverride(name, content string) error {
	cleanName, ok := sanitizeConfigFileName(name)
	if !ok {
		return fmt.Errorf("invalid file name")
	}

	var parsed any
	if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	customDir := filepath.Join(b.orch.StateDir(), "custom-configs")
	if err := os.MkdirAll(customDir, 0755); err != nil {
		return fmt.Errorf("failed to create custom-configs directory: %w", err)
	}

	safePath := filepath.Join(customDir, cleanName)

	//nolint:gosec // Config file permissions are intentionally 0644; name is sanitized
	if err := os.WriteFile(safePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write override: %w", err)
	}

	if err := b.orch.GenerateConfigs(context.Background()); err != nil {
		b.log.WithError(err).Error("Failed to regenerate configs after override save")
	}

	return nil
}

// DeleteConfigFileOverride removes a custom override for a config file.
func (b *labBackend) DeleteConfigFileOverride(name string) error {
	cleanName, ok := sanitizeConfigFileName(name)
	if !ok {
		return fmt.Errorf("invalid file name")
	}

	safePath := filepath.Join(
		b.orch.StateDir(), "custom-configs", cleanName,
	)

	if err := os.Remove(safePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove override: %w", err)
	}

	if err := b.orch.GenerateConfigs(context.Background()); err != nil {
		b.log.WithError(err).Error("Failed to regenerate configs after override delete")
	}

	return nil
}

// Regenerate triggers config regeneration.
func (b *labBackend) Regenerate(ctx context.Context) error {
	return b.orch.GenerateConfigs(ctx)
}

// RecreateOrchestrator rebuilds the orchestrator with the current config.
func (b *labBackend) RecreateOrchestrator() error {
	newOrch, err := orchestrator.NewOrchestrator(b.log, b.labCfg, b.cfgPath)
	if err != nil {
		return fmt.Errorf("failed to recreate orchestrator: %w", err)
	}

	b.orch = newOrch
	b.wrapper.SetOrchestrator(newOrch)

	return nil
}

// StateDir returns the lab stack's state directory.
func (b *labBackend) StateDir() string {
	return b.orch.StateDir()
}

// RedisAddr returns the Redis address for the Lab stack.
func (b *labBackend) RedisAddr() string {
	return fmt.Sprintf("localhost:%d", b.labCfg.Infrastructure.RedisPort)
}
