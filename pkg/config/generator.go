package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigType represents different config types we generate.
type ConfigType string

const (
	ConfigTypeTransformation ConfigType = "transformation"
	ConfigTypeScheduled      ConfigType = "scheduled"
	ConfigTypeExternal       ConfigType = "external"
)

// GenerateOrUseCustomConfig checks for custom config, generates if missing, writes result
// This replaces the 3x copy-pasted pattern in orchestrator.go generateConfigs()
//
// Flow:
//
//  1. Check if custom config exists in customConfigsDir
//  2. If yes, copy custom config to output
//  3. If no, call generateFunc() and write result to output
//
// Parameters:
//   - configType: transformation, scheduled, or external
//   - network: network name (mainnet, sepolia, etc)
//   - customConfigsDir: directory to check for custom configs (e.g., ".xcli/custom-configs")
//   - outputConfigsDir: directory to write final config (e.g., ".xcli/configs")
//   - generateFunc: function that generates default config content
func GenerateOrUseCustomConfig(
	configType ConfigType,
	network string,
	customConfigsDir string,
	outputConfigsDir string,
	generateFunc func() ([]byte, error),
) error {
	customPath := filepath.Join(customConfigsDir, fmt.Sprintf("%s_%s.yaml", configType, network))
	outputPath := filepath.Join(outputConfigsDir, fmt.Sprintf("%s_%s.yaml", configType, network))

	// Check for custom config first
	if _, err := os.Stat(customPath); err == nil {
		// Custom config exists, copy it
		content, err := os.ReadFile(customPath)
		if err != nil {
			return fmt.Errorf("failed to read custom config %s: %w", customPath, err)
		}

		if err := os.WriteFile(outputPath, content, 0600); err != nil {
			return fmt.Errorf("failed to write custom config to %s: %w", outputPath, err)
		}

		fmt.Printf("Using custom %s config for network %s\n", configType, network)

		return nil
	}

	// No custom config, generate default
	content, err := generateFunc()
	if err != nil {
		return fmt.Errorf("failed to generate %s config for network %s: %w", configType, network, err)
	}

	if err := os.WriteFile(outputPath, content, 0600); err != nil {
		return fmt.Errorf("failed to write generated config to %s: %w", outputPath, err)
	}

	fmt.Printf("Generated default %s config for network %s\n", configType, network)

	return nil
}
