package configtui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Colors.
var (
	colorGreen  = lipgloss.Color("10")
	colorRed    = lipgloss.Color("9")
	colorYellow = lipgloss.Color("11")
	colorCyan   = lipgloss.Color("14")
	colorGray   = lipgloss.Color("8")
	colorWhite  = lipgloss.Color("15")
)

// Styles.
var (
	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCyan).
			MarginBottom(1)

	styleEnabled = lipgloss.NewStyle().
			Foreground(colorGreen)

	styleDisabled = lipgloss.NewStyle().
			Foreground(colorRed)

	styleSelected = lipgloss.NewStyle().
			Background(lipgloss.Color("237")).
			Foreground(colorWhite)

	stylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorCyan).
			Padding(0, 1)

	stylePanelActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorYellow).
				Padding(0, 1)

	styleHelp = lipgloss.NewStyle().
			Foreground(colorWhite).
			Bold(true).
			Background(lipgloss.Color("#3A3A3A")).
			Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorCyan).
			Bold(true).
			Background(lipgloss.Color("#3A3A3A")).
			Padding(0, 1)

	styleDirty = lipgloss.NewStyle().
			Foreground(colorYellow).
			Bold(true)

	styleEnvLabel = lipgloss.NewStyle().
			Foreground(colorGray)

	styleEnvValue = lipgloss.NewStyle().
			Foreground(colorWhite)

	styleNeeded = lipgloss.NewStyle().
			Foreground(colorYellow).
			Bold(true)

	styleSectionHeader = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorCyan).
				Underline(true)
)

// View renders the TUI.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var sb strings.Builder

	// Title.
	title := styleTitle.Render("xcli CBT Model Configuration")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	// Env section.
	sb.WriteString(m.renderEnvSection())
	sb.WriteString("\n")

	// Model panels (side by side).
	sb.WriteString(m.renderModelPanels())
	sb.WriteString("\n")

	// Help bar.
	sb.WriteString(m.renderHelp())
	sb.WriteString("\n")

	// Status bar.
	sb.WriteString(m.renderStatusBar())

	return sb.String()
}

// Env var descriptions.
const (
	envTimestampDesc = "Consensus layer backfill to (unix timestamp, 0 for unlimited)"
	envBlockDesc     = "Execution layer backfill to (block number, 0 for unlimited)"
)

// renderEnvSection renders the environment variables section.
func (m Model) renderEnvSection() string {
	var sb strings.Builder

	isActive := m.activeSection == sectionEnv
	header := styleSectionHeader.Render("ENVIRONMENT VARIABLES")
	sb.WriteString(header)
	sb.WriteString("\n")

	// EXTERNAL_MODEL_MIN_TIMESTAMP.
	row1 := m.renderEnvRow(0, "EXTERNAL_MODEL_MIN_TIMESTAMP", m.envMinTimestamp, m.envTimestampEnabled, isActive, envTimestampDesc)
	sb.WriteString(row1)
	sb.WriteString("\n")

	// EXTERNAL_MODEL_MIN_BLOCK.
	row2 := m.renderEnvRow(1, "EXTERNAL_MODEL_MIN_BLOCK", m.envMinBlock, m.envBlockEnabled, isActive, envBlockDesc)
	sb.WriteString(row2)

	return sb.String()
}

// renderEnvRow renders a single environment variable row with description.
func (m Model) renderEnvRow(index int, name, value string, enabled, sectionActive bool, description string) string {
	checkbox := "[ ]"
	if enabled {
		checkbox = "[x]"
	}

	displayValue := value
	if displayValue == "" {
		displayValue = ""
	}

	// Show cursor when this field is selected.
	if sectionActive && m.selectedIndex == index {
		displayValue = displayValue + "_"
	}

	// Show placeholder if empty and not selected.
	if displayValue == "" {
		displayValue = "(empty)"
	}

	line := fmt.Sprintf("%s %s: %s  %s",
		checkbox,
		styleEnvLabel.Render(name),
		styleEnvValue.Render(displayValue),
		styleEnvLabel.Render("- "+description))

	if sectionActive && m.selectedIndex == index {
		return styleSelected.Render(line)
	}

	return line
}

// renderModelPanels renders the two model panels side by side.
func (m Model) renderModelPanels() string {
	// Calculate panel width (roughly half the terminal, or a reasonable default).
	panelWidth := 40
	if m.width > 0 {
		panelWidth = (m.width - 6) / 2 // Account for borders and spacing
		if panelWidth < 30 {
			panelWidth = 30
		}
	}

	// Determine visible height for model lists - stretch to fill available space.
	// Reserve space for: title (3), env section (4), help bar (2), status bar (1), panel borders (2).
	reservedHeight := 12
	visibleHeight := 15 // Default if no height info

	if m.height > 0 {
		visibleHeight = m.height - reservedHeight
		if visibleHeight < 5 {
			visibleHeight = 5
		}
	}

	// Render external models panel.
	externalContent := m.renderModelList(m.externalModels, m.externalScroll, visibleHeight,
		m.activeSection == sectionExternal, panelWidth-4)
	externalHeader := fmt.Sprintf("EXTERNAL MODELS (%d)", len(m.externalModels))

	// Calculate panel height (content height + header + padding).
	panelHeight := visibleHeight + 3

	var externalPanel string
	if m.activeSection == sectionExternal {
		externalPanel = stylePanelActive.Width(panelWidth).Height(panelHeight).Render(externalHeader + "\n" + externalContent)
	} else {
		externalPanel = stylePanel.Width(panelWidth).Height(panelHeight).Render(externalHeader + "\n" + externalContent)
	}

	// Render transformation models panel.
	transformContent := m.renderModelList(m.transformationModels, m.transformScroll, visibleHeight,
		m.activeSection == sectionTransformation, panelWidth-4)
	transformHeader := fmt.Sprintf("TRANSFORMATION MODELS (%d)", len(m.transformationModels))

	var transformPanel string
	if m.activeSection == sectionTransformation {
		transformPanel = stylePanelActive.Width(panelWidth).Height(panelHeight).Render(transformHeader + "\n" + transformContent)
	} else {
		transformPanel = stylePanel.Width(panelWidth).Height(panelHeight).Render(transformHeader + "\n" + transformContent)
	}

	// Join panels horizontally (transformations on left, external on right).
	return lipgloss.JoinHorizontal(lipgloss.Top, transformPanel, "  ", externalPanel)
}

// renderModelList renders a list of models with scrolling.
func (m Model) renderModelList(models []ModelEntry, scroll, visibleHeight int, isActive bool, width int) string {
	if len(models) == 0 {
		return styleEnvLabel.Render("(no models found)")
	}

	var lines []string

	// Determine visible range.
	start := scroll
	end := scroll + visibleHeight

	if end > len(models) {
		end = len(models)
	}

	for i := start; i < end; i++ {
		model := models[i]
		line := m.renderModelEntry(model, i, isActive, width)
		lines = append(lines, line)
	}

	// Add scroll indicators if needed.
	result := strings.Join(lines, "\n")

	if scroll > 0 {
		result = styleEnvLabel.Render("▲ (more above)") + "\n" + result
	}

	if end < len(models) {
		result = result + "\n" + styleEnvLabel.Render("▼ (more below)")
	}

	return result
}

// renderModelEntry renders a single model entry.
func (m Model) renderModelEntry(model ModelEntry, index int, isActive bool, maxWidth int) string {
	checkbox := "[ ]"
	checkStyle := styleDisabled

	// Check if this disabled model is needed by an enabled model.
	isNeeded := !model.Enabled && m.IsModelNeededByEnabled(model.Name)

	if model.Enabled {
		checkbox = "[x]"
		checkStyle = styleEnabled
	} else if isNeeded {
		// Highlight as warning - disabled but needed.
		checkStyle = styleNeeded
	}

	// Truncate name if too long (account for warning indicator).
	name := model.Name
	maxNameLen := maxWidth - 5 // Account for checkbox and spacing

	if isNeeded {
		maxNameLen -= 3 // Account for warning indicator.
	}

	if maxNameLen < 10 {
		maxNameLen = 10
	}

	if len(name) > maxNameLen {
		name = name[:maxNameLen-2] + ".."
	}

	// Add warning indicator for needed but disabled models.
	line := fmt.Sprintf("%s %s", checkStyle.Render(checkbox), name)
	if isNeeded {
		line += " " + styleNeeded.Render("!")
	}

	if isActive && m.selectedIndex == index {
		return styleSelected.Render(line)
	}

	return line
}

// renderHelp renders the help bar.
func (m Model) renderHelp() string {
	var help string

	if m.confirmQuit {
		help = "[y] Confirm quit  [n] Cancel"
	} else {
		switch m.activeSection {
		case sectionEnv:
			help = "[←/→] Switch section  [↑/↓] Navigate  [0-9] Type value  [Backspace] Delete  [s] Save  [q] Quit"
		default:
			help = "[←/→] Switch section  [↑/↓] Navigate  [Space] Toggle  [a] All on  [n] All off  [d] Enable deps  [s] Save  [q] Quit"
		}
	}

	width := m.width
	if width < 80 {
		width = 80
	}

	return styleHelp.Width(width).Render(help)
}

// renderStatusBar renders the status bar.
func (m Model) renderStatusBar() string {
	status := fmt.Sprintf("Section: %s", m.activeSection)

	if m.dirty {
		status += "  " + styleDirty.Render("* Unsaved changes")
	}

	if m.statusMsg != "" {
		status += "  " + m.statusMsg
	}

	width := m.width
	if width < 80 {
		width = 80
	}

	return styleStatusBar.Width(width).Render(status)
}
