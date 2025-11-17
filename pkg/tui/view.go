package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/acarl005/stripansi"
	"github.com/charmbracelet/lipgloss"
)

// View renders the TUI
func (m Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	var sections []string

	// Title
	title := StyleTitle.Render("xcli Lab Stack Dashboard")
	sections = append(sections, title)

	// Services panel (full width, no infrastructure panel)
	servicesPanel := m.renderServicesPanel()
	sections = append(sections, servicesPanel)

	// Logs panel
	logsPanel := m.renderLogsPanel()
	sections = append(sections, logsPanel)

	// Help footer
	help := m.renderHelp()
	sections = append(sections, help)

	// Status bar
	status := m.renderStatusBar()
	sections = append(sections, status)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
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
		if i == m.selectedIndex && m.activePanel == "services" {
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
	// Full width now (no infrastructure panel)
	return StylePanel.Width(m.width - 4).Height(panelHeight).Render(content)
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
	help := "[↑/↓] Navigate  [s] Start  [t] Stop  [r] Restart  [u/d] Scroll  [g] Jump to Latest  [q] Quit"
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
	timestamp := line.Timestamp.Format("15:04:05")
	level := line.Level

	// Color by level
	switch level {
	case "ERROR", "ERRO":
		level = StyleError.Render(level)
	case "WARN":
		level = lipgloss.NewStyle().Foreground(ColorYellow).Render(level)
	case "INFO":
		level = lipgloss.NewStyle().Foreground(ColorBlue).Render(level)
	}

	return fmt.Sprintf("%s [%s] %s", timestamp, level, line.Message)
}
