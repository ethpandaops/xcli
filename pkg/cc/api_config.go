package cc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ethpandaops/xcli/pkg/config"
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
	DefaultEnabled       *bool                `json:"defaultEnabled,omitempty"`
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
	Name        string `json:"name"`
	OverrideKey string `json:"overrideKey"`
	Enabled     bool   `json:"enabled"`
}

// cbtOverridesRequest is the request body for saving CBT overrides.
type cbtOverridesRequest struct {
	DefaultEnabled       *bool                `json:"defaultEnabled,omitempty"`
	ExternalModels       []modelEntryResponse `json:"externalModels"`
	TransformationModels []modelEntryResponse `json:"transformationModels"`
	EnvMinTimestamp      string               `json:"envMinTimestamp"`
	EnvTimestampEnabled  bool                 `json:"envTimestampEnabled"`
	EnvMinBlock          string               `json:"envMinBlock"`
	EnvBlockEnabled      bool                 `json:"envBlockEnabled"`
}

// handleGetLabConfig returns the editable config via the backend.
func (a *apiHandler) handleGetLabConfig(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	resp, err := a.backend.GetEditableConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})

		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handlePutLabConfig updates the config via the backend.
func (a *apiHandler) handlePutLabConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("failed to read body: %v", err),
		})

		return
	}

	a.mu.Lock()

	if err := a.backend.PutEditableConfig(json.RawMessage(body)); err != nil {
		a.mu.Unlock()

		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})

		return
	}

	a.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleGetConfigFiles lists generated config files via the backend.
func (a *apiHandler) handleGetConfigFiles(
	w http.ResponseWriter,
	_ *http.Request,
) {
	files, err := a.backend.GetConfigFiles()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})

		return
	}

	writeJSON(w, http.StatusOK, files)
}

// handleGetConfigFile returns the raw content of a generated config file.
func (a *apiHandler) handleGetConfigFile(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	resp, err := a.backend.GetConfigFile(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": err.Error(),
		})

		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handlePutConfigFileOverride saves a custom override for a config file.
func (a *apiHandler) handlePutConfigFileOverride(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	var req struct {
		Content string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})

		return
	}

	if err := a.backend.PutConfigFileOverride(name, req.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})

		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleDeleteConfigFileOverride removes a custom override for a config file.
func (a *apiHandler) handleDeleteConfigFileOverride(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.backend.DeleteConfigFileOverride(name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})

		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleGetOverrides returns the CBT overrides state via the backend.
func (a *apiHandler) handleGetOverrides(
	w http.ResponseWriter,
	_ *http.Request,
) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	resp, err := a.backend.GetOverrides()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})

		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handlePutOverrides saves CBT overrides via the backend.
func (a *apiHandler) handlePutOverrides(
	w http.ResponseWriter,
	r *http.Request,
) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("failed to read body: %v", err),
		})

		return
	}

	a.mu.Lock()

	if err := a.backend.PutOverrides(json.RawMessage(body)); err != nil {
		a.mu.Unlock()

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})

		return
	}

	a.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handlePostRegenerate triggers config regeneration via the backend.
func (a *apiHandler) handlePostRegenerate(
	w http.ResponseWriter,
	r *http.Request,
) {
	if err := a.backend.Regenerate(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf(
				"failed to regenerate configs: %v", err,
			),
		})

		return
	}

	a.sseHub.Broadcast("config_regenerated", map[string]string{
		"status": "ok",
	})

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// sanitizeConfigFileName validates and cleans a config file name from user input.
// Returns the cleaned base name and true if valid, or empty string and false.
func sanitizeConfigFileName(name string) (string, bool) {
	if name == "" || strings.Contains(name, "..") || strings.Contains(name, "/") {
		return "", false
	}

	return filepath.Base(filepath.Clean(name)), true
}
