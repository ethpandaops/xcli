package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// MenuAction represents an action in the context menu.
type MenuAction struct {
	Key         string // Shortcut key (e.g., "s", "t", "r", "b")
	Label       string // Display label (e.g., "Start", "Stop")
	Description string // Brief description
	Enabled     bool   // Whether the action is currently available
}

// GetMenuActions returns the available actions for a service based on its status.
func GetMenuActions(status string) []MenuAction {
	isRunning := status == statusRunning
	isStopped := status == statusStopped

	return []MenuAction{
		{
			Key:         "s",
			Label:       "Start",
			Description: "Start the service",
			Enabled:     isStopped,
		},
		{
			Key:         "t",
			Label:       "Stop",
			Description: "Stop the service",
			Enabled:     isRunning,
		},
		{
			Key:         "r",
			Label:       "Restart",
			Description: "Restart the service",
			Enabled:     true,
		},
		{
			Key:         "b",
			Label:       "Rebuild",
			Description: "Rebuild and restart",
			Enabled:     true,
		},
		{
			Key:         "l",
			Label:       "Logs (new window)",
			Description: "Open logs in new terminal",
			Enabled:     isRunning,
		},
	}
}

// Styles for the menu.
var (
	StyleMenuBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorCyan).
			Padding(1, 2)

	StyleMenuTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan).
			MarginBottom(1)

	StyleMenuEnabled = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")) // White

	StyleMenuDisabled = lipgloss.NewStyle().
				Foreground(ColorGray)

	StyleMenuKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorGreen)

	StyleMenuKeyDisabled = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorGray)
)

// RenderMenu renders the context menu overlay.
func RenderMenu(serviceName string, actions []MenuAction) string {
	// Pre-allocate: title + actions + blank line + footer
	rows := make([]string, 0, len(actions)+3)

	// Title
	title := StyleMenuTitle.Render(fmt.Sprintf("Actions: %s", serviceName))
	rows = append(rows, title)

	// Actions
	for _, action := range actions {
		var row string

		if action.Enabled {
			key := StyleMenuKey.Render(fmt.Sprintf("[%s]", action.Key))
			label := StyleMenuEnabled.Render(action.Label)
			row = fmt.Sprintf("  %s %s", key, label)
		} else {
			key := StyleMenuKeyDisabled.Render(fmt.Sprintf("[%s]", action.Key))
			label := StyleMenuDisabled.Render(action.Label)
			row = fmt.Sprintf("  %s %s", key, label)
		}

		rows = append(rows, row)
	}

	// Footer
	rows = append(rows, "")
	footer := StyleMenuDisabled.Render("  [Esc] Cancel")
	rows = append(rows, footer)

	content := strings.Join(rows, "\n")

	return StyleMenuBorder.Render(content)
}
