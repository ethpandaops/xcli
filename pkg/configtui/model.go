package configtui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Section constants for navigation.
const (
	sectionEnv            = "env"
	sectionExternal       = "external"
	sectionTransformation = "transformation"
)

// ModelEntry represents a single model (external or transformation).
type ModelEntry struct {
	Name    string
	Enabled bool // true = default (omit from overrides), false = disabled
}

// Model is the Bubbletea application state.
type Model struct {
	// Config paths.
	xatuCBTPath   string
	overridesPath string

	// Data.
	externalModels       []ModelEntry
	transformationModels []ModelEntry
	existingOverrides    *CBTOverrides // Original loaded overrides for preserving config blocks.

	// Dependencies: model -> list of models it depends on (direct deps only).
	dependencies map[string][]string

	// Environment variables.
	envMinTimestamp     string // Value (empty = use default)
	envMinBlock         string // Value (empty = use default)
	envTimestampEnabled bool   // Whether to include in output (uncommented)
	envBlockEnabled     bool   // Whether to include in output (uncommented)

	// UI State.
	activeSection   string // "env", "external", "transformation"
	selectedIndex   int    // Current cursor position within section
	externalScroll  int    // Scroll offset for external models
	transformScroll int    // Scroll offset for transformation models

	// Dimensions.
	width  int
	height int

	// Status.
	dirty     bool   // Has unsaved changes
	statusMsg string // Status message to display
	quitting  bool   // Whether we're quitting

	// Confirm quit state.
	confirmQuit bool
}

// NewModel creates a new Model with initial state.
func NewModel(xatuCBTPath, overridesPath string) Model {
	return Model{
		xatuCBTPath:          xatuCBTPath,
		overridesPath:        overridesPath,
		externalModels:       make([]ModelEntry, 0),
		transformationModels: make([]ModelEntry, 0),
		activeSection:        sectionTransformation, // Start on left panel
		selectedIndex:        0,
	}
}

// Init is called when the program starts.
func (m Model) Init() tea.Cmd {
	return nil
}

// GetCurrentSectionModels returns the models for the currently active section.
func (m *Model) GetCurrentSectionModels() []ModelEntry {
	switch m.activeSection {
	case sectionExternal:
		return m.externalModels
	case sectionTransformation:
		return m.transformationModels
	default:
		return nil
	}
}

// GetCurrentScroll returns the scroll offset for the current section.
func (m *Model) GetCurrentScroll() int {
	switch m.activeSection {
	case sectionExternal:
		return m.externalScroll
	case sectionTransformation:
		return m.transformScroll
	default:
		return 0
	}
}

// SetCurrentScroll sets the scroll offset for the current section.
func (m *Model) SetCurrentScroll(offset int) {
	switch m.activeSection {
	case sectionExternal:
		m.externalScroll = offset
	case sectionTransformation:
		m.transformScroll = offset
	}
}

// ToggleCurrentModel toggles the enabled state of the currently selected model.
func (m *Model) ToggleCurrentModel() {
	switch m.activeSection {
	case sectionExternal:
		if m.selectedIndex < len(m.externalModels) {
			m.externalModels[m.selectedIndex].Enabled = !m.externalModels[m.selectedIndex].Enabled
			m.dirty = true
		}
	case sectionTransformation:
		if m.selectedIndex < len(m.transformationModels) {
			m.transformationModels[m.selectedIndex].Enabled = !m.transformationModels[m.selectedIndex].Enabled
			m.dirty = true
		}
	}
}

// EnableAllInSection enables all models in the current section.
func (m *Model) EnableAllInSection() {
	switch m.activeSection {
	case sectionExternal:
		for i := range m.externalModels {
			m.externalModels[i].Enabled = true
		}

		m.dirty = true
	case sectionTransformation:
		for i := range m.transformationModels {
			m.transformationModels[i].Enabled = true
		}

		m.dirty = true
	}
}

// DisableAllInSection disables all models in the current section.
func (m *Model) DisableAllInSection() {
	switch m.activeSection {
	case sectionExternal:
		for i := range m.externalModels {
			m.externalModels[i].Enabled = false
		}

		m.dirty = true
	case sectionTransformation:
		for i := range m.transformationModels {
			m.transformationModels[i].Enabled = false
		}

		m.dirty = true
	}
}

// EnableAllModels enables all models in both external and transformation sections.
func (m *Model) EnableAllModels() {
	for i := range m.externalModels {
		m.externalModels[i].Enabled = true
	}

	for i := range m.transformationModels {
		m.transformationModels[i].Enabled = true
	}

	m.dirty = true
}

// DisableAllModels disables all models in both external and transformation sections.
func (m *Model) DisableAllModels() {
	for i := range m.externalModels {
		m.externalModels[i].Enabled = false
	}

	for i := range m.transformationModels {
		m.transformationModels[i].Enabled = false
	}

	m.dirty = true
}

// GetSectionLength returns the number of items in the current section.
func (m *Model) GetSectionLength() int {
	switch m.activeSection {
	case sectionEnv:
		return 2 // Two env vars
	case sectionExternal:
		return len(m.externalModels)
	case sectionTransformation:
		return len(m.transformationModels)
	default:
		return 0
	}
}

// NextSection moves to the next section.
// Order: env -> transformation (left panel) -> external (right panel) -> env.
func (m *Model) NextSection() {
	switch m.activeSection {
	case sectionEnv:
		m.activeSection = sectionTransformation
	case sectionTransformation:
		m.activeSection = sectionExternal
	case sectionExternal:
		m.activeSection = sectionEnv
	}

	m.selectedIndex = 0
}

// PrevSection moves to the previous section.
// Order: env -> external (right panel) -> transformation (left panel) -> env.
func (m *Model) PrevSection() {
	switch m.activeSection {
	case sectionEnv:
		m.activeSection = sectionExternal
	case sectionExternal:
		m.activeSection = sectionTransformation
	case sectionTransformation:
		m.activeSection = sectionEnv
	}

	m.selectedIndex = 0
}

// EnableMissingDependencies enables all disabled models that are needed by enabled models.
// Returns the number of models enabled.
func (m *Model) EnableMissingDependencies() int {
	count := 0

	// Check external models.
	for i := range m.externalModels {
		if !m.externalModels[i].Enabled && m.IsModelNeededByEnabled(m.externalModels[i].Name) {
			m.externalModels[i].Enabled = true
			count++
		}
	}

	// Check transformation models.
	for i := range m.transformationModels {
		if !m.transformationModels[i].Enabled && m.IsModelNeededByEnabled(m.transformationModels[i].Name) {
			m.transformationModels[i].Enabled = true
			count++
		}
	}

	if count > 0 {
		m.dirty = true
	}

	return count
}

// IsModelNeededByEnabled checks if a disabled model is needed by any enabled model.
// Returns true if the model is a dependency of at least one enabled model.
func (m *Model) IsModelNeededByEnabled(modelName string) bool {
	if m.dependencies == nil {
		return false
	}

	// Check all enabled transformation models.
	for _, tm := range m.transformationModels {
		if !tm.Enabled {
			continue
		}

		// Check if this enabled model depends on the given model.
		deps := m.dependencies[tm.Name]
		for _, dep := range deps {
			if dep == modelName {
				return true
			}
		}
	}

	// Also check enabled external models (they might depend on other externals via transformations).
	for _, em := range m.externalModels {
		if !em.Enabled {
			continue
		}

		deps := m.dependencies[em.Name]
		for _, dep := range deps {
			if dep == modelName {
				return true
			}
		}
	}

	return false
}
