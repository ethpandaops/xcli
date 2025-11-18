package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/acarl005/stripansi"
	"github.com/charmbracelet/lipgloss"
)

// View renders the TUI.
func (m Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	var sections []string

	// Title
	title := StyleTitle.Render("xcli Lab Stack Dashboard")
	sections = append(sections, title)

	// Services panel and right side panel (activity + actions) side by side
	servicesPanel := m.renderServicesPanel()
	rightPanel := m.renderRightPanel()

	// Join services and right panel horizontally
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, servicesPanel, rightPanel)
	sections = append(sections, topRow)

	// Logs panel
	logsPanel := m.renderLogsPanel()
	sections = append(sections, logsPanel)

	// Help footer
	help := m.renderHelp()
	sections = append(sections, help)

	// Status bar
	status := m.renderStatusBar()
	sections = append(sections, status)

	mainView := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Overlay menu if visible
	if m.showMenu && m.selectedIndex < len(m.services) {
		serviceName := m.services[m.selectedIndex].Name
		menuContent := RenderMenu(serviceName, m.menuActions)

		// Place menu centered on screen
		return lipgloss.Place(
			m.width,
			m.height,
			lipgloss.Center,
			lipgloss.Center,
			menuContent,
		)
	}

	return mainView
}

func (m Model) renderServicesPanel() string {
	rows := make([]string, 0, len(m.services)+2)
	rows = append(rows, "SERVICE                STATUS      UPTIME       HEALTH")
	rows = append(rows, strings.Repeat("─", 60))

	for i, svc := range m.services {
		status := svc.Status
		health := m.health[svc.Name].Status
		uptime := formatDuration(svc.Uptime)

		// Format the row with proper alignment (no colors yet)
		// Then apply colors to individual cells
		serviceName := fmt.Sprintf("%-22s", svc.Name)

		// Status column - fixed width to align uptime
		var statusStr string
		if status == statusRunning {
			statusStr = StyleRunning.Render(status) + strings.Repeat(" ", 11-len(status))
		} else {
			statusStr = StyleStopped.Render(status) + strings.Repeat(" ", 11-len(status))
		}

		// Uptime column - right-aligned for better readability
		uptimeStr := fmt.Sprintf("%8s", uptime)

		// Health column
		healthStr := health
		switch health {
		case "healthy":
			healthStr = StyleRunning.Render("✓ " + health)
		case "unhealthy":
			healthStr = StyleError.Render("✗ " + health)
		}

		row := fmt.Sprintf("%s %s %s     %s",
			serviceName, statusStr, uptimeStr, healthStr)

		// Highlight selected
		if i == m.selectedIndex && m.activePanel == panelServices {
			row = StyleSelected.Render(row)
		}

		rows = append(rows, row)
	}

	content := strings.Join(rows, "\n")
	// Reduced height for top panels (was m.height/2-4, now much smaller)
	panelHeight := len(m.services) + 4 // Header + separator + services + padding
	if panelHeight > 12 {
		panelHeight = 12 // Cap at 12 lines
	}

	// Services panel takes 2/3 of width
	panelWidth := (m.width * 2 / 3) - 2

	return StylePanel.Width(panelWidth).Height(panelHeight).Render(content)
}

func (m Model) renderRightPanel() string {
	// Right panel takes 1/3 of width
	panelWidth := (m.width / 3) - 2

	// Calculate height to match services panel
	panelHeight := len(m.services) + 4
	if panelHeight > 12 {
		panelHeight = 12
	}

	var rows []string

	// Activity section
	if m.activity != "" {
		elapsed := time.Since(m.activityStart).Round(time.Second)

		// Spinner frames
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frameIndex := int(elapsed.Seconds()) % len(spinnerFrames)
		spinner := spinnerFrames[frameIndex]

		titleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorYellow)

		spinnerStyle := lipgloss.NewStyle().
			Foreground(ColorYellow)

		elapsedStyle := lipgloss.NewStyle().
			Foreground(ColorGray)

		rows = append(rows, titleStyle.Render("ACTIVITY"))
		rows = append(rows, strings.Repeat("─", 24))
		rows = append(rows, spinnerStyle.Render(spinner)+" "+m.activity)
		rows = append(rows, elapsedStyle.Render(fmt.Sprintf("Elapsed: %s", elapsed)))
	} else {
		titleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorGray)

		rows = append(rows, titleStyle.Render("ACTIVITY"))
		rows = append(rows, strings.Repeat("─", 24))
		rows = append(rows, lipgloss.NewStyle().Foreground(ColorGray).Render("Idle"))
	}

	// Spacer
	rows = append(rows, "")

	// Actions section
	actionsTitleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorCyan)

	rows = append(rows, actionsTitleStyle.Render("ACTIONS"))
	rows = append(rows, strings.Repeat("─", 24))

	// Rebuild All button
	var buttonStyle lipgloss.Style

	if m.activePanel == panelActions {
		// Selected/focused state
		buttonStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("0")).
			Background(ColorCyan).
			Padding(0, 1)
	} else {
		// Normal state
		buttonStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("238")).
			Padding(0, 1)
	}

	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorGreen)

	if m.activePanel == panelActions {
		keyStyle = keyStyle.Foreground(lipgloss.Color("0"))
	}

	button := buttonStyle.Render(keyStyle.Render("R") + " Rebuild All")
	rows = append(rows, button)

	content := strings.Join(rows, "\n")

	return StylePanel.Width(panelWidth).Height(panelHeight).Render(content)
}

func (m Model) renderLogsPanel() string {
	rows := make([]string, 0, 100)

	if m.selectedIndex < len(m.services) {
		selectedService := m.services[m.selectedIndex].Name

		header := fmt.Sprintf("LOGS: %s", selectedService)
		if m.followMode {
			header += " [LIVE]"
		}

		rows = append(rows, header)
		rows = append(rows, strings.Repeat("─", 100))

		logs := m.logs[selectedService]

		// Calculate available height: total height - title - top panels - help - status - padding
		logPanelHeight := m.height - 20 // Reserve 20 lines for other UI elements
		if logPanelHeight < 10 {
			logPanelHeight = 10 // Minimum height
		}

		// In follow mode, always show the most recent logs
		var start, end int

		if m.followMode {
			// Show the last N lines (auto-scroll to bottom)
			if len(logs) > logPanelHeight-2 {
				start = len(logs) - (logPanelHeight - 2)
			} else {
				start = 0
			}

			end = len(logs)
		} else {
			// Manual scroll mode - use logScroll offset
			start = m.logScroll

			end = start + logPanelHeight - 2
			if end > len(logs) {
				end = len(logs)
			}
		}

		if start < len(logs) && start >= 0 {
			maxWidth := m.width - 8 // Leave padding for panel borders

			for _, line := range logs[start:end] {
				formatted := formatLogLine(line)
				// Truncate line if too long to prevent wrapping
				// Use visual length (without ANSI codes) for comparison
				visualLen := len(stripansi.Strip(formatted))
				if visualLen > maxWidth {
					// Truncate and add ellipsis
					truncated := ""
					currentLen := 0

					for _, r := range formatted {
						truncated += string(r)
						// Only count non-ANSI characters
						if r != '\x1b' {
							currentLen++
						}

						if currentLen >= maxWidth-3 {
							break
						}
					}

					formatted = truncated + "..."
				}

				rows = append(rows, formatted)
			}
		}
	}

	content := strings.Join(rows, "\n")
	// Calculate height dynamically
	logPanelHeight := m.height - 20
	if logPanelHeight < 10 {
		logPanelHeight = 10
	}

	return StylePanel.Width(m.width - 4).Height(logPanelHeight).Render(content)
}

func (m Model) renderHelp() string {
	help := "[↑/↓] Navigate  [Tab] Switch Panel  [Enter] Select  [r] Rebuild  [u/d] Scroll  [q] Quit"

	return StyleHelp.Render(help)
}

func (m Model) renderStatusBar() string {
	cfg := m.wrapper.orch.Config()
	mode := cfg.Mode
	networks := cfg.EnabledNetworks()
	lastUpdate := m.lastUpdate.Format("15:04:05")

	// Format network names nicely
	networkNames := make([]string, len(networks))
	for i, net := range networks {
		networkNames[i] = net.Name
	}

	networksStr := strings.Join(networkNames, ", ")
	if networksStr == "" {
		networksStr = "none"
	}

	status := fmt.Sprintf("Mode: %s | Networks: %s | Updated: %s",
		mode, networksStr, lastUpdate)

	return StyleStatusBar.Render(status)
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "-"
	}

	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}

	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}

	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func formatLogLine(line LogLine) string {
	// Color the level indicator
	var levelIndicator string

	switch line.Level {
	case "ERROR", "ERRO":
		levelIndicator = StyleError.Render("E")
	case "WARN", "WARNING":
		levelIndicator = lipgloss.NewStyle().Foreground(ColorYellow).Render("W")
	case "DEBUG":
		levelIndicator = lipgloss.NewStyle().Foreground(ColorGray).Render("D")
	default:
		levelIndicator = lipgloss.NewStyle().Foreground(ColorBlue).Render("I")
	}

	// Show level indicator + full log line
	return fmt.Sprintf("%s %s", levelIndicator, line.Message)
}
