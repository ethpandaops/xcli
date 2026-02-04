package configtui

import (
	"fmt"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// Key constants.
const keyEsc = "esc"

// Update handles all messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		return m, nil
	}

	return m, nil
}

// handleKeyPress handles keyboard input.
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle confirm quit state.
	if m.confirmQuit {
		return m.handleConfirmQuit(msg)
	}

	// Handle filter input mode (transformation section only).
	if m.filterMode {
		return m.handleFilterInput(msg)
	}

	// Handle env section - direct typing for values.
	if m.activeSection == sectionEnv {
		if handled, newModel, cmd := m.handleEnvInput(msg); handled {
			return newModel, cmd
		}
	}

	// Normal key handling.
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true

		return m, tea.Quit

	case "q":
		if m.dirty {
			m.confirmQuit = true

			return m, nil
		}

		m.quitting = true

		return m, tea.Quit

	case "tab":
		m.NextSection()

		return m, nil

	case "shift+tab":
		m.PrevSection()

		return m, nil

	case "up", "k":
		return m.handleUp(), nil

	case "down", "j":
		return m.handleDown(), nil

	case "left", "h":
		m.PrevSection()

		return m, nil

	case "right", "l":
		m.NextSection()

		return m, nil

	case " ", "enter":
		return m.handleToggle(), nil

	case "a":
		if m.activeSection != sectionEnv {
			m.EnableAllModels()
			m.statusMsg = "Enabled all models"

			return m, nil
		}

	case "n":
		if m.activeSection != sectionEnv {
			m.DisableAllModels()
			m.statusMsg = "Disabled all models"

			return m, nil
		}

	case "d":
		count := m.EnableMissingDependencies()
		if count > 0 {
			m.statusMsg = fmt.Sprintf("Enabled %d missing dependencies", count)
		} else {
			m.statusMsg = "No missing dependencies"
		}

		return m, nil

	case "s":
		err := SaveOverrides(m.overridesPath, &m, m.existingOverrides)
		if err != nil {
			m.statusMsg = "Error: " + err.Error()

			return m, nil
		}

		m.dirty = false
		m.statusMsg = "Saved to " + m.overridesPath

		return m, nil

	case "r":
		// Reload from file.
		return m.reload()

	case "/":
		// Enter filter mode (transformation section only).
		if m.activeSection == sectionTransformation {
			m.filterMode = true
			m.filterInput = ""

			return m, nil
		}

	case keyEsc:
		// Clear active filter (transformation section only).
		if m.activeSection == sectionTransformation && m.filterText != "" {
			m.ClearFilter()

			return m, nil
		}
	}

	return m, nil
}

// handleFilterInput handles keyboard input during filter mode.
func (m Model) handleFilterInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc:
		// Cancel filter mode without applying.
		m.filterMode = false
		m.filterInput = ""

		return m, nil

	case "enter":
		// Apply filter (or clear if empty).
		m.filterMode = false
		if m.filterInput == "" {
			m.ClearFilter()
		} else {
			m.ApplyTransformFilter()
		}

		return m, nil

	case "backspace":
		// Delete last character.
		if len(m.filterInput) > 0 {
			m.filterInput = m.filterInput[:len(m.filterInput)-1]
		}

		return m, nil

	default:
		// Append printable characters.
		for _, r := range msg.String() {
			if r >= 32 && r < 127 {
				m.filterInput += string(r)
			}
		}

		return m, nil
	}
}

// handleConfirmQuit handles the confirm quit dialog.
func (m Model) handleConfirmQuit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.quitting = true

		return m, tea.Quit
	case "n", "N", keyEsc:
		m.confirmQuit = false

		return m, nil
	}

	return m, nil
}

// handleEnvInput handles direct typing in env var fields.
// Returns (handled, model, cmd) - if handled is true, the input was processed.
func (m Model) handleEnvInput(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	key := msg.String()

	// Handle backspace - delete last character.
	if key == "backspace" {
		if m.selectedIndex == 0 && len(m.envMinTimestamp) > 0 {
			m.envMinTimestamp = m.envMinTimestamp[:len(m.envMinTimestamp)-1]
			m.envTimestampEnabled = m.envMinTimestamp != ""
			m.dirty = true

			return true, m, nil
		} else if m.selectedIndex == 1 && len(m.envMinBlock) > 0 {
			m.envMinBlock = m.envMinBlock[:len(m.envMinBlock)-1]
			m.envBlockEnabled = m.envMinBlock != ""
			m.dirty = true

			return true, m, nil
		}

		return true, m, nil
	}

	// Handle digit input - collect all digits from the input (supports paste).
	var digits string

	for _, r := range key {
		if unicode.IsDigit(r) {
			digits += string(r)
		}
	}

	if digits != "" {
		if m.selectedIndex == 0 {
			m.envMinTimestamp += digits
			m.envTimestampEnabled = true
		} else {
			m.envMinBlock += digits
			m.envBlockEnabled = true
		}

		m.dirty = true

		return true, m, nil
	}

	// Not handled - let normal key handling take over.
	return false, m, nil
}

// handleUp moves the cursor up.
func (m Model) handleUp() Model {
	if m.selectedIndex > 0 {
		m.selectedIndex--

		// Adjust scroll if needed.
		scroll := m.GetCurrentScroll()
		if m.selectedIndex < scroll {
			m.SetCurrentScroll(m.selectedIndex)
		}
	}

	return m
}

// handleDown moves the cursor down.
func (m Model) handleDown() Model {
	maxIndex := m.GetSectionLength() - 1
	if m.selectedIndex < maxIndex {
		m.selectedIndex++

		// Adjust scroll if needed (assuming visible height of ~15).
		visibleHeight := 15
		scroll := m.GetCurrentScroll()

		if m.selectedIndex >= scroll+visibleHeight {
			m.SetCurrentScroll(m.selectedIndex - visibleHeight + 1)
		}
	}

	return m
}

// handleToggle toggles the current item.
func (m Model) handleToggle() Model {
	switch m.activeSection {
	case sectionEnv:
		// Toggle env var enabled state.
		if m.selectedIndex == 0 {
			m.envTimestampEnabled = !m.envTimestampEnabled
		} else {
			m.envBlockEnabled = !m.envBlockEnabled
		}

		m.dirty = true
	default:
		m.ToggleCurrentModel()
	}

	return m
}

// reload reloads the configuration from file.
func (m Model) reload() (Model, tea.Cmd) {
	overrides, fileExists, err := LoadOverrides(m.overridesPath)
	if err != nil {
		m.statusMsg = "Error reloading: " + err.Error()

		return m, nil
	}

	m.existingOverrides = overrides

	// Reset env vars.
	m.envMinTimestamp = overrides.Models.Env["EXTERNAL_MODEL_MIN_TIMESTAMP"]
	m.envMinBlock = overrides.Models.Env["EXTERNAL_MODEL_MIN_BLOCK"]
	m.envTimestampEnabled = m.envMinTimestamp != ""
	m.envBlockEnabled = m.envMinBlock != ""

	// Reset model enabled states.
	// If no file exists, default all models to disabled.
	for i := range m.externalModels {
		m.externalModels[i].Enabled = fileExists && !IsModelDisabled(overrides, m.externalModels[i].Name)
	}

	for i := range m.transformationModels {
		m.transformationModels[i].Enabled = fileExists && !IsModelDisabled(overrides, m.transformationModels[i].Name)
	}

	m.dirty = false
	m.statusMsg = "Reloaded from file"

	return m, nil
}
