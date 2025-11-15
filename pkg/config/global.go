package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// GlobalConfig represents the global xcli configuration stored in ~/.xcli/config.yaml.
type GlobalConfig struct {
	XCLIPath string `yaml:"xcliPath,omitempty"`
}

// LoadGlobalConfig loads the global config from ~/.xcli/config.yaml.
func LoadGlobalConfig() (*GlobalConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	globalConfigPath := filepath.Join(homeDir, ".xcli", "config.yaml")

	// If file doesn't exist, return empty config
	if _, statErr := os.Stat(globalConfigPath); os.IsNotExist(statErr) {
		return &GlobalConfig{}, nil
	}

	data, err := os.ReadFile(globalConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read global config: %w", err)
	}

	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}

	return &cfg, nil
}

// SaveGlobalConfig saves the global config to ~/.xcli/config.yaml.
func SaveGlobalConfig(cfg *GlobalConfig) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	globalConfigDir := filepath.Join(homeDir, ".xcli")
	if mkdirErr := os.MkdirAll(globalConfigDir, 0755); mkdirErr != nil {
		return fmt.Errorf("failed to create global config directory: %w", err)
	}

	globalConfigPath := filepath.Join(globalConfigDir, "config.yaml")

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal global config: %w", err)
	}

	//nolint:gosec // Config file permissions are intentionally 0644 for readability
	if err := os.WriteFile(globalConfigPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write global config: %w", err)
	}

	return nil
}

// SetXCLIPath sets the xcli installation path in the global config.
func SetXCLIPath(path string) error {
	// Make path absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Verify the path has a .xcli.yaml file
	configPath := filepath.Join(absPath, ".xcli.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("no .xcli.yaml found in %s", absPath)
	}

	cfg := &GlobalConfig{
		XCLIPath: absPath,
	}

	return SaveGlobalConfig(cfg)
}
