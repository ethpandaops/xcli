package configtui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ethpandaops/xcli/pkg/seeddata"
)

// Run starts the config TUI.
func Run(xatuCBTPath, overridesPath string) error {
	// Check if terminal is a TTY.
	if !isatty() {
		return fmt.Errorf("config TUI requires an interactive terminal")
	}

	// Discover models.
	externalModels, transformModels, err := DiscoverModels(xatuCBTPath)
	if err != nil {
		return fmt.Errorf("failed to discover models: %w", err)
	}

	// Load existing overrides.
	overrides, fileExists, err := LoadOverrides(overridesPath)
	if err != nil {
		return fmt.Errorf("failed to load overrides: %w", err)
	}

	// Create the model.
	m := NewModel(xatuCBTPath, overridesPath)
	m.existingOverrides = overrides

	// Initialize external models.
	// If no overrides file exists, default all models to disabled.
	m.externalModels = make([]ModelEntry, 0, len(externalModels))
	for _, name := range externalModels {
		overrideKey := name
		if db := seeddata.GetExternalModelDatabase(name, xatuCBTPath); db != "" {
			overrideKey = db + "." + name
		}

		enabled := fileExists && !IsModelDisabled(overrides, overrideKey)
		m.externalModels = append(m.externalModels, ModelEntry{
			Name:        name,
			OverrideKey: overrideKey,
			Enabled:     enabled,
		})
	}

	// Initialize transformation models.
	// If no overrides file exists, default all models to disabled.
	m.transformationModels = make([]ModelEntry, 0, len(transformModels))
	for _, name := range transformModels {
		enabled := fileExists && !IsModelDisabled(overrides, name)
		m.transformationModels = append(m.transformationModels, ModelEntry{
			Name:        name,
			OverrideKey: name,
			Enabled:     enabled,
		})
	}

	// Initialize env vars from loaded overrides.
	m.envMinTimestamp = overrides.Models.Env["EXTERNAL_MODEL_MIN_TIMESTAMP"]
	m.envMinBlock = overrides.Models.Env["EXTERNAL_MODEL_MIN_BLOCK"]
	m.envTimestampEnabled = m.envMinTimestamp != ""
	m.envBlockEnabled = m.envMinBlock != ""

	// Load model dependencies for dependency warnings.
	m.dependencies = LoadDependencies(xatuCBTPath, transformModels)

	// Run the TUI.
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	_, err = p.Run()
	if err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	return nil
}

// isatty checks if stdout is a terminal.
func isatty() bool {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// DiscoverModels discovers external and transformation models from the xatu-cbt repo.
func DiscoverModels(xatuCBTPath string) (external []string, transformation []string, err error) {
	// Discover external models.
	externalDir := filepath.Join(xatuCBTPath, "models", "external")

	entries, err := os.ReadDir(externalDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read external models directory: %w", err)
	}

	external = make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".sql") {
			external = append(external, strings.TrimSuffix(name, ".sql"))
		}
	}

	sort.Strings(external)

	// Discover transformation models.
	transformDir := filepath.Join(xatuCBTPath, "models", "transformations")

	entries, err = os.ReadDir(transformDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read transformations directory: %w", err)
	}

	transformation = make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Support .sql, .yml, and .yaml extensions.
		for _, ext := range []string{".sql", ".yml", ".yaml"} {
			if strings.HasSuffix(name, ext) {
				transformation = append(transformation, strings.TrimSuffix(name, ext))

				break
			}
		}
	}

	sort.Strings(transformation)

	return external, transformation, nil
}

// LoadDependencies loads the dependency graph for all transformation models.
// Returns a map of model name -> list of all dependencies (recursive, flattened).
func LoadDependencies(xatuCBTPath string, transformModels []string) map[string][]string {
	deps := make(map[string][]string, len(transformModels))

	for _, model := range transformModels {
		tree, err := seeddata.ResolveDependencyTree(model, xatuCBTPath, nil)
		if err != nil {
			// Skip models with dependency resolution errors.
			continue
		}

		// Get all dependencies (external and intermediate).
		allDeps := append(tree.GetExternalDependencies(), tree.GetIntermediateDependencies()...)
		deps[model] = allDeps
	}

	return deps
}
