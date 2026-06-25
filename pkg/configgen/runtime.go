package configgen

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/constants"
)

// GenerateRuntimeConfigs writes all generated service configs for the bound
// runtime under <root>/.xcli/instances/<id>/configs.
func (g *Generator) GenerateRuntimeConfigs() (string, error) {
	if g.runtime == nil {
		return "", fmt.Errorf("runtime is required")
	}

	if g.runtime.Workspace == nil {
		return "", fmt.Errorf("runtime workspace is required")
	}

	if g.runtime.Manifest == nil {
		return "", fmt.Errorf("runtime manifest is required")
	}

	configsDir := filepath.Join(g.runtime.Manifest.StateDir, constants.DirConfigs)
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create configs directory: %w", err)
	}

	customConfigsDir := filepath.Join(g.runtime.Workspace.StateDir, constants.DirCustomConfigs)
	userOverridesPath := g.runtime.Workspace.OverridesPath

	for _, network := range g.cfg.EnabledNetworks() {
		cbtFilename := fmt.Sprintf(constants.ConfigFileCBT, network.Name)
		cbtPath := filepath.Join(configsDir, cbtFilename)

		if hasCustomConfig, customPath := findCustomConfig(customConfigsDir, cbtFilename); hasCustomConfig {
			if err := copyFile(customPath, cbtPath); err != nil {
				return "", fmt.Errorf("failed to copy custom CBT config for %s: %w", network.Name, err)
			}
		} else {
			cbtConfig, err := g.GenerateCBTConfig(network.Name, userOverridesPath)
			if err != nil {
				return "", fmt.Errorf("failed to generate CBT config for %s: %w", network.Name, err)
			}

			//nolint:gosec // Config files are intentionally readable.
			if err := os.WriteFile(cbtPath, []byte(cbtConfig), 0644); err != nil {
				return "", fmt.Errorf("failed to write CBT config: %w", err)
			}
		}

		apiFilename := fmt.Sprintf(constants.ConfigFileCBTAPI, network.Name)
		apiPath := filepath.Join(configsDir, apiFilename)

		if hasCustomConfig, customPath := findCustomConfig(customConfigsDir, apiFilename); hasCustomConfig {
			if err := copyFile(customPath, apiPath); err != nil {
				return "", fmt.Errorf("failed to copy custom cbt-api config for %s: %w", network.Name, err)
			}
		} else {
			apiConfig, err := g.GenerateCBTAPIConfig(network.Name)
			if err != nil {
				return "", fmt.Errorf("failed to generate cbt-api config for %s: %w", network.Name, err)
			}

			//nolint:gosec // Config files are intentionally readable.
			if err := os.WriteFile(apiPath, []byte(apiConfig), 0644); err != nil {
				return "", fmt.Errorf("failed to write cbt-api config: %w", err)
			}
		}
	}

	backendFilename := constants.ConfigFileLabBackend
	backendPath := filepath.Join(configsDir, backendFilename)

	if hasCustomConfig, customPath := findCustomConfig(customConfigsDir, backendFilename); hasCustomConfig {
		if err := copyFile(customPath, backendPath); err != nil {
			return "", fmt.Errorf("failed to copy custom lab-backend config: %w", err)
		}
	} else {
		backendConfig, err := g.GenerateLabBackendConfig(userOverridesPath)
		if err != nil {
			return "", fmt.Errorf("failed to generate lab-backend config: %w", err)
		}

		//nolint:gosec // Config files are intentionally readable.
		if err := os.WriteFile(backendPath, []byte(backendConfig), 0644); err != nil {
			return "", fmt.Errorf("failed to write lab-backend config: %w", err)
		}
	}

	if g.cfg.Infrastructure.Observability.Enabled {
		if _, err := g.GeneratePrometheusConfig(configsDir); err != nil {
			return "", fmt.Errorf("failed to generate Prometheus config: %w", err)
		}

		if err := g.GenerateGrafanaProvisioning(configsDir, g.runtime.Workspace.StateDir); err != nil {
			return "", fmt.Errorf("failed to generate Grafana provisioning: %w", err)
		}
	}

	return configsDir, nil
}

func findCustomConfig(customConfigsDir, filename string) (bool, string) {
	path := filepath.Join(customConfigsDir, filename)
	if _, err := os.Stat(path); err == nil {
		return true, path
	}

	return false, ""
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", src, err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	//nolint:gosec // Config files are intentionally readable.
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", dst, err)
	}

	return nil
}
