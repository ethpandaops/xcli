package cc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/configtui"
	"github.com/ethpandaops/xcli/pkg/constants"
)

// maskedPassword is the placeholder shown for non-empty passwords.
const maskedPassword = "********"

// labConfigResponse is the full editable lab config (passwords masked).
type labConfigResponse struct {
	Mode           string                      `json:"mode"`
	Networks       []config.NetworkConfig      `json:"networks"`
	Infrastructure config.InfrastructureConfig `json:"infrastructure"`
	Ports          config.LabPortsConfig       `json:"ports"`
	Dev            config.LabDevConfig         `json:"dev"`
	Repos          config.LabReposConfig       `json:"repos"`
}

// configFileInfo describes a generated config file.
type configFileInfo struct {
	Name        string `json:"name"`
	HasOverride bool   `json:"hasOverride"`
	Size        int64  `json:"size"`
}

// configFileContent holds the content of a config file and override info.
type configFileContent struct {
	Name            string `json:"name"`
	Content         string `json:"content"`
	HasOverride     bool   `json:"hasOverride"`
	OverrideContent string `json:"overrideContent,omitempty"`
}

// cbtOverridesResponse is the CBT overrides state for the UI.
type cbtOverridesResponse struct {
	ExternalModels       []modelEntryResponse `json:"externalModels"`
	TransformationModels []modelEntryResponse `json:"transformationModels"`
	Dependencies         map[string][]string  `json:"dependencies"`
	EnvMinTimestamp      string               `json:"envMinTimestamp"`
	EnvTimestampEnabled  bool                 `json:"envTimestampEnabled"`
	EnvMinBlock          string               `json:"envMinBlock"`
	EnvBlockEnabled      bool                 `json:"envBlockEnabled"`
}

// modelEntryResponse is a model with enabled status.
type modelEntryResponse struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// cbtOverridesRequest is the request body for saving CBT overrides.
type cbtOverridesRequest struct {
	ExternalModels       []modelEntryResponse `json:"externalModels"`
	TransformationModels []modelEntryResponse `json:"transformationModels"`
	EnvMinTimestamp      string               `json:"envMinTimestamp"`
	EnvTimestampEnabled  bool                 `json:"envTimestampEnabled"`
	EnvMinBlock          string               `json:"envMinBlock"`
	EnvBlockEnabled      bool                 `json:"envBlockEnabled"`
}

// sanitizeConfigFileName validates and cleans a config file name from user input.
// Returns the cleaned base name and true if valid, or empty string and false.
func sanitizeConfigFileName(name string) (string, bool) {
	if name == "" || strings.Contains(name, "..") || strings.Contains(name, "/") {
		return "", false
	}

	return filepath.Base(filepath.Clean(name)), true
}

// handleGetLabConfig returns the editable lab config with passwords masked.
func (a *apiHandler) handleGetLabConfig(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	cfg := a.labCfg
	a.mu.RUnlock()

	resp := labConfigResponse{
		Mode:           cfg.Mode,
		Networks:       cfg.Networks,
		Infrastructure: cfg.Infrastructure,
		Ports:          cfg.Ports,
		Dev:            cfg.Dev,
		Repos:          cfg.Repos,
	}

	// Mask passwords.
	if resp.Infrastructure.ClickHouse.Xatu.ExternalPassword != "" {
		resp.Infrastructure.ClickHouse.Xatu.ExternalPassword = maskedPassword
	}

	if resp.Infrastructure.ClickHouse.CBT.ExternalPassword != "" {
		resp.Infrastructure.ClickHouse.CBT.ExternalPassword = maskedPassword
	}

	writeJSON(w, http.StatusOK, resp)
}

// handlePutLabConfig updates the lab config, validates, saves, and regenerates.
func (a *apiHandler) handlePutLabConfig(w http.ResponseWriter, r *http.Request) {
	var req labConfigResponse

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})

		return
	}

	a.mu.Lock()

	// Preserve masked passwords.
	if req.Infrastructure.ClickHouse.Xatu.ExternalPassword == maskedPassword {
		req.Infrastructure.ClickHouse.Xatu.ExternalPassword =
			a.labCfg.Infrastructure.ClickHouse.Xatu.ExternalPassword
	}

	if req.Infrastructure.ClickHouse.CBT.ExternalPassword == maskedPassword {
		req.Infrastructure.ClickHouse.CBT.ExternalPassword =
			a.labCfg.Infrastructure.ClickHouse.CBT.ExternalPassword
	}

	// Build updated config for validation.
	updated := &config.LabConfig{
		Mode:           req.Mode,
		Networks:       req.Networks,
		Infrastructure: req.Infrastructure,
		Ports:          req.Ports,
		Dev:            req.Dev,
		Repos:          req.Repos,
		TUI:            a.labCfg.TUI, // Preserve TUI settings.
	}

	if err := updated.Validate(); err != nil {
		a.mu.Unlock()

		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("validation failed: %v", err),
		})

		return
	}

	// Apply to live config.
	*a.labCfg = *updated

	a.mu.Unlock()

	// Save to disk.
	fullCfg := &config.Config{Lab: a.labCfg}

	if err := fullCfg.Save(a.cfgPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to save config: %v", err),
		})

		return
	}

	// Recreate orchestrator so it picks up the new config (mode, ports, etc.).
	a.mu.Lock()

	if err := a.recreateOrchestrator(); err != nil {
		a.mu.Unlock()
		a.log.WithError(err).Error("Failed to recreate orchestrator after config update")

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to apply config: %v", err),
		})

		return
	}

	a.mu.Unlock()

	// Regenerate configs.
	var regenErr string

	if err := a.orch.GenerateConfigs(r.Context()); err != nil {
		a.log.WithError(err).Error(
			"Failed to regenerate configs after config update",
		)

		regenErr = err.Error()
	}

	resp := map[string]string{"status": "ok"}
	if regenErr != "" {
		resp["regenerateError"] = regenErr
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleGetConfigFiles lists generated config files with override status.
func (a *apiHandler) handleGetConfigFiles(
	w http.ResponseWriter,
	_ *http.Request,
) {
	configsDir := filepath.Join(a.orch.StateDir(), "configs")

	entries, err := os.ReadDir(configsDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf(
				"failed to read configs directory: %v", err,
			),
		})

		return
	}

	customDir := filepath.Join(a.orch.StateDir(), "custom-configs")
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

	writeJSON(w, http.StatusOK, files)
}

// handleGetConfigFile returns the raw content of a generated config file.
func (a *apiHandler) handleGetConfigFile(
	w http.ResponseWriter,
	r *http.Request,
) {
	name, ok := sanitizeConfigFileName(r.PathValue("name"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid file name",
		})

		return
	}

	configsDir := filepath.Join(a.orch.StateDir(), "configs")
	safePath := filepath.Join(configsDir, name)

	content, err := os.ReadFile(safePath) //nolint:gosec // name is sanitized above
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("config file not found: %v", err),
		})

		return
	}

	resp := configFileContent{
		Name:    name,
		Content: string(content),
	}

	// Check for override.
	customPath := filepath.Join(
		a.orch.StateDir(), "custom-configs", name,
	)

	overrideContent, overrideErr := os.ReadFile(customPath) //nolint:gosec // name is sanitized above
	if overrideErr == nil {
		resp.HasOverride = true
		resp.OverrideContent = string(overrideContent)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handlePutConfigFileOverride saves a custom override for a config file.
func (a *apiHandler) handlePutConfigFileOverride(
	w http.ResponseWriter,
	r *http.Request,
) {
	name, ok := sanitizeConfigFileName(r.PathValue("name"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid file name",
		})

		return
	}

	var req struct {
		Content string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})

		return
	}

	// Validate YAML syntax.
	var parsed any
	if err := yaml.Unmarshal([]byte(req.Content), &parsed); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid YAML: %v", err),
		})

		return
	}

	customDir := filepath.Join(a.orch.StateDir(), "custom-configs")
	if err := os.MkdirAll(customDir, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf(
				"failed to create custom-configs directory: %v", err,
			),
		})

		return
	}

	safePath := filepath.Join(customDir, name)

	//nolint:gosec // Config file permissions are intentionally 0644; name is sanitized
	if err := os.WriteFile(safePath, []byte(req.Content), 0644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to write override: %v", err),
		})

		return
	}

	// Regenerate configs.
	var regenErr string

	if err := a.orch.GenerateConfigs(r.Context()); err != nil {
		a.log.WithError(err).Error(
			"Failed to regenerate configs after override save",
		)

		regenErr = err.Error()
	}

	resp := map[string]string{"status": "ok"}
	if regenErr != "" {
		resp["regenerateError"] = regenErr
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleDeleteConfigFileOverride removes a custom override for a config file.
func (a *apiHandler) handleDeleteConfigFileOverride(
	w http.ResponseWriter,
	r *http.Request,
) {
	name, ok := sanitizeConfigFileName(r.PathValue("name"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid file name",
		})

		return
	}

	safePath := filepath.Join(
		a.orch.StateDir(), "custom-configs", name,
	)

	if err := os.Remove(safePath); err != nil && !os.IsNotExist(err) { //nolint:gosec // name is sanitized above
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to remove override: %v", err),
		})

		return
	}

	// Regenerate configs.
	var regenErr string

	if err := a.orch.GenerateConfigs(r.Context()); err != nil {
		a.log.WithError(err).Error(
			"Failed to regenerate configs after override delete",
		)

		regenErr = err.Error()
	}

	resp := map[string]string{"status": "ok"}
	if regenErr != "" {
		resp["regenerateError"] = regenErr
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleGetOverrides returns the CBT overrides state.
func (a *apiHandler) handleGetOverrides(
	w http.ResponseWriter,
	_ *http.Request,
) {
	a.mu.RLock()
	xatuCBTPath := a.labCfg.Repos.XatuCBT
	stateDir := a.orch.StateDir()
	a.mu.RUnlock()

	overridesPath := filepath.Join(
		filepath.Dir(stateDir), constants.CBTOverridesFile,
	)

	// Discover available models.
	externalNames, transformNames, err := configtui.DiscoverModels(
		xatuCBTPath,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to discover models: %v", err),
		})

		return
	}

	// Load existing overrides.
	overrides, fileExists, err := configtui.LoadOverrides(overridesPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to load overrides: %v", err),
		})

		return
	}

	// Load dependency graph.
	deps := configtui.LoadDependencies(xatuCBTPath, transformNames)

	// Build response with merged enabled status.
	resp := cbtOverridesResponse{
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
		enabled := fileExists && !configtui.IsModelDisabled(overrides, name)

		resp.ExternalModels = append(
			resp.ExternalModels, modelEntryResponse{
				Name:    name,
				Enabled: enabled,
			},
		)
	}

	for _, name := range transformNames {
		enabled := fileExists && !configtui.IsModelDisabled(overrides, name)

		resp.TransformationModels = append(
			resp.TransformationModels,
			modelEntryResponse{
				Name:    name,
				Enabled: enabled,
			},
		)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handlePutOverrides saves CBT overrides.
func (a *apiHandler) handlePutOverrides(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req cbtOverridesRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})

		return
	}

	a.mu.RLock()
	stateDir := a.orch.StateDir()
	a.mu.RUnlock()

	overridesPath := filepath.Join(
		filepath.Dir(stateDir), constants.CBTOverridesFile,
	)

	// Load existing overrides to preserve config blocks.
	existingOverrides, _, err := configtui.LoadOverrides(overridesPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf(
				"failed to load existing overrides: %v", err,
			),
		})

		return
	}

	// Convert request entries to configtui.ModelEntry.
	externalModels := make(
		[]configtui.ModelEntry, 0, len(req.ExternalModels),
	)

	for _, m := range req.ExternalModels {
		externalModels = append(externalModels, configtui.ModelEntry{
			Name:    m.Name,
			Enabled: m.Enabled,
		})
	}

	transformModels := make(
		[]configtui.ModelEntry, 0, len(req.TransformationModels),
	)

	for _, m := range req.TransformationModels {
		transformModels = append(transformModels, configtui.ModelEntry{
			Name:    m.Name,
			Enabled: m.Enabled,
		})
	}

	// Save overrides.
	if err := configtui.SaveOverridesFromEntries(
		overridesPath,
		externalModels,
		transformModels,
		req.EnvMinTimestamp,
		req.EnvTimestampEnabled,
		req.EnvMinBlock,
		req.EnvBlockEnabled,
		existingOverrides,
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to save overrides: %v", err),
		})

		return
	}

	// Regenerate configs.
	var regenErr string

	if err := a.orch.GenerateConfigs(r.Context()); err != nil {
		a.log.WithError(err).Error(
			"Failed to regenerate configs after overrides save",
		)

		regenErr = err.Error()
	}

	resp := map[string]string{"status": "ok"}
	if regenErr != "" {
		resp["regenerateError"] = regenErr
	}

	writeJSON(w, http.StatusOK, resp)
}

// handlePostRegenerate triggers config regeneration.
func (a *apiHandler) handlePostRegenerate(
	w http.ResponseWriter,
	r *http.Request,
) {
	if err := a.orch.GenerateConfigs(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf(
				"failed to regenerate configs: %v", err,
			),
		})

		return
	}

	// Broadcast SSE event.
	a.sseHub.Broadcast("config_regenerated", map[string]string{
		"status": "ok",
	})

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}
