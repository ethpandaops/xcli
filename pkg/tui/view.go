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

	sections := make([]string, 0, 5)

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

	// Overlay log detail if visible
	if m.logDetailMode {
		detailContent := m.renderLogDetail()

		return lipgloss.Place(
			m.width,
			m.height,
			lipgloss.Center,
			lipgloss.Center,
			detailContent,
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
		case healthHealthy:
			healthStr = StyleRunning.Render("✓ " + health)
		case healthUnhealthy:
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
	if panelHeight > servicesPanelMaxHeight {
		panelHeight = servicesPanelMaxHeight
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
	if panelHeight > servicesPanelMaxHeight {
		panelHeight = servicesPanelMaxHeight
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
		logs := m.logs[selectedService]

		// Apply log level filter
		logs = m.filterLogsByLevel(logs)

		// Apply regex filter if active
		if m.filterActive && m.filterRegex != "" {
			logs = m.filterLogs(logs)
		}

		// Calculate available height using shared helper
		logPanelHeight := m.getLogPanelHeight()

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

		// Build header with status indicators
		header := fmt.Sprintf("LOGS: %s", selectedService)

		if m.followMode {
			header += " [LIVE]"
		} else {
			// Show indicator when paused with more logs below
			logsBelow := len(logs) - end
			if logsBelow > 0 {
				header += fmt.Sprintf(" [PAUSED ↓%d more]", logsBelow)
			} else {
				header += " [PAUSED]"
			}
		}

		// Show log level filter if not ALL
		if m.logLevelFilter != LogLevelAll {
			header += fmt.Sprintf(" [LEVEL: %s+]", m.logLevelFilter)
		}

		if m.filterActive {
			if m.filterError != nil {
				header += fmt.Sprintf(" [INVALID REGEX: %s]", m.filterRegex)
			} else {
				header += fmt.Sprintf(" [FILTER: %s]", m.filterRegex)
			}
		}

		rows = append(rows, header)
		rows = append(rows, strings.Repeat("─", 100))

		// Filter bar (if in filter mode)
		if m.filterMode {
			filterPrompt := "Filter (regex): " + m.filterInput + "█"
			filterStyle := lipgloss.NewStyle().
				Foreground(ColorCyan).
				Bold(true)
			rows = append(rows, filterStyle.Render(filterPrompt))
			rows = append(rows, strings.Repeat("─", 100))
		}

		if start < len(logs) && start >= 0 {
			maxWidth := m.width - 8 // Leave padding for panel borders

			for _, line := range logs[start:end] {
				formatted := m.formatLogLineWithHighlight(line)
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

	return StylePanel.Width(m.width - 4).Height(m.getLogPanelHeight()).Render(content)
}

func (m Model) renderHelp() string {
	var help string

	if m.logDetailMode {
		help = "[↑/↓] Navigate Logs  [Esc] Close Detail"
	} else if m.filterMode {
		help = "[Enter] Apply Filter  [Esc] Cancel  [Type to enter regex pattern]"
	} else if m.filterActive {
		if m.followMode {
			help = "[↑/↓] Navigate  [Tab] Switch  [f] Filter  [l] Level  [Esc] Clear  [u/d] Scroll  [q] Quit"
		} else {
			help = "[↑/↓] Navigate  [Tab] Switch  [f] Filter  [l] Level  [Esc] Clear  [u/d] Scroll  [g] Follow  [q] Quit"
		}
	} else {
		if m.followMode {
			help = "[↑/↓] Navigate  [Tab] Switch  [Enter] Select  [f] Filter  [l] Level  [r] Rebuild  [u/d] Scroll  [q] Quit"
		} else {
			help = "[↑/↓] Navigate  [Tab] Switch  [Enter] Select  [f] Filter  [l] Level  [r] Rebuild  [u/d] Scroll  [g] Follow  [q] Quit"
		}
	}

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

// formatLogLineWithHighlight formats a log line, highlighting filter matches if active.
func (m Model) formatLogLineWithHighlight(line LogLine) string {
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

	message := line.Message

	// Highlight filter matches if filter is active
	if m.filterActive && m.filterCompiled != nil {
		message = m.highlightMatches(message)
	}

	return fmt.Sprintf("%s %s", levelIndicator, message)
}

// highlightMatches highlights all regex matches in the text with a distinct style.
func (m Model) highlightMatches(text string) string {
	if m.filterCompiled == nil {
		return text
	}

	highlightStyle := lipgloss.NewStyle().
		Background(ColorYellow).
		Foreground(lipgloss.Color("0")). // Black text
		Bold(true)

	// Find all match indices
	matches := m.filterCompiled.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	// Build result string with highlights
	var result strings.Builder

	lastEnd := 0

	for _, match := range matches {
		start, end := match[0], match[1]

		// Add text before match
		if start > lastEnd {
			result.WriteString(text[lastEnd:start])
		}

		// Add highlighted match
		result.WriteString(highlightStyle.Render(text[start:end]))
		lastEnd = end
	}

	// Add remaining text after last match
	if lastEnd < len(text) {
		result.WriteString(text[lastEnd:])
	}

	return result.String()
}

// filterLogs filters log lines using the pre-compiled regex.
func (m Model) filterLogs(logs []LogLine) []LogLine {
	if m.filterCompiled == nil {
		return logs
	}

	filtered := make([]LogLine, 0, len(logs))
	for _, log := range logs {
		if m.filterCompiled.MatchString(log.Message) || m.filterCompiled.MatchString(log.Raw) {
			filtered = append(filtered, log)
		}
	}

	return filtered
}

// filterLogsByLevel filters log lines based on the current log level filter.
// Each level includes itself and all higher severity levels.
func (m Model) filterLogsByLevel(logs []LogLine) []LogLine {
	if m.logLevelFilter == LogLevelAll {
		return logs
	}

	filtered := make([]LogLine, 0, len(logs))

	for _, log := range logs {
		if m.matchesLogLevel(log.Level) {
			filtered = append(filtered, log)
		}
	}

	return filtered
}

// matchesLogLevel checks if a log level matches the current filter.
// Each filter level includes itself and all higher severity levels.
func (m Model) matchesLogLevel(level string) bool {
	// Normalize level variations
	normalizedLevel := level
	switch level {
	case "ERRO":
		normalizedLevel = LogLevelError
	case "WARNING":
		normalizedLevel = LogLevelWarn
	}

	switch m.logLevelFilter {
	case LogLevelError:
		return normalizedLevel == LogLevelError
	case LogLevelWarn:
		return normalizedLevel == LogLevelError || normalizedLevel == LogLevelWarn
	case LogLevelInfo:
		return normalizedLevel == LogLevelError || normalizedLevel == LogLevelWarn || normalizedLevel == LogLevelInfo
	case LogLevelDebug:
		// DEBUG shows everything (same as ALL)
		return true
	default:
		return true
	}
}

// renderLogDetail renders the log detail overlay showing the full log line.
func (m Model) renderLogDetail() string {
	if m.selectedIndex >= len(m.services) {
		return ""
	}

	selectedService := m.services[m.selectedIndex].Name
	logs := m.logs[selectedService]

	// Apply log level filter
	logs = m.filterLogsByLevel(logs)

	// Apply regex filter if active
	if m.filterActive && m.filterCompiled != nil {
		logs = m.filterLogs(logs)
	}

	// Calculate visible window using shared helper
	logPanelHeight := m.getLogPanelHeight()

	var start int

	if m.followMode {
		if len(logs) > logPanelHeight-2 {
			start = len(logs) - (logPanelHeight - 2)
		}
	} else {
		start = m.logScroll
	}

	// Get the selected log line
	logIndex := start + m.selectedLogIndex
	if logIndex >= len(logs) || logIndex < 0 {
		return ""
	}

	selectedLog := logs[logIndex]

	// Calculate widths based on terminal width
	panelWidth := m.width - 4          // Account for margins
	contentWidth := panelWidth - 6     // Account for border and padding
	separatorWidth := contentWidth - 2 // Slightly smaller for visual padding

	// Build the detail view
	var rows []string

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorCyan)

	rows = append(rows, titleStyle.Render("LOG DETAIL"))
	rows = append(rows, strings.Repeat("─", separatorWidth))
	rows = append(rows, "")

	// Metadata
	metaStyle := lipgloss.NewStyle().Foreground(ColorGray)
	rows = append(rows, metaStyle.Render(fmt.Sprintf("Service:   %s", selectedLog.Service)))
	rows = append(rows, metaStyle.Render(fmt.Sprintf("Timestamp: %s", selectedLog.Timestamp.Format("2006-01-02 15:04:05.000"))))
	rows = append(rows, metaStyle.Render(fmt.Sprintf("Level:     %s", selectedLog.Level)))
	rows = append(rows, "")
	rows = append(rows, strings.Repeat("─", separatorWidth))
	rows = append(rows, "")

	// Full message with word wrap
	rows = append(rows, titleStyle.Render("Message:"))
	wrapped := wrapText(selectedLog.Message, contentWidth)

	// Apply highlighting to each wrapped line if filter is active
	if m.filterActive && m.filterCompiled != nil {
		for i, line := range wrapped {
			wrapped[i] = m.highlightMatches(line)
		}
	}

	rows = append(rows, wrapped...)
	rows = append(rows, "")

	// Raw log if different from message
	if selectedLog.Raw != "" && selectedLog.Raw != selectedLog.Message {
		rows = append(rows, strings.Repeat("─", separatorWidth))
		rows = append(rows, "")
		rows = append(rows, titleStyle.Render("Raw:"))
		wrappedRaw := wrapText(selectedLog.Raw, contentWidth)

		// Apply highlighting to each wrapped line if filter is active
		if m.filterActive && m.filterCompiled != nil {
			for i, line := range wrappedRaw {
				wrappedRaw[i] = m.highlightMatches(line)
			}
		}

		rows = append(rows, wrappedRaw...)
	}

	rows = append(rows, "")
	rows = append(rows, strings.Repeat("─", separatorWidth))

	helpStyle := lipgloss.NewStyle().Foreground(ColorGray).Italic(true)
	rows = append(rows, helpStyle.Render("[↑/↓] Navigate  [c/y] Copy  [Esc] Close"))

	content := strings.Join(rows, "\n")

	// Create panel style - full width
	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorCyan).
		Padding(1, 2).
		Width(panelWidth)

	return panelStyle.Render(content)
}

// wrapText wraps text to the specified width.
func wrapText(text string, width int) []string {
	if len(text) <= width {
		return []string{text}
	}

	var (
		lines       []string
		currentLine string
	)

	words := strings.Fields(text)
	for _, word := range words {
		if len(currentLine)+len(word)+1 > width {
			if currentLine != "" {
				lines = append(lines, currentLine)
				currentLine = word
			} else {
				// Word is longer than width, force break
				for len(word) > width {
					lines = append(lines, word[:width])
					word = word[width:]
				}

				currentLine = word
			}
		} else {
			if currentLine != "" {
				currentLine += " " + word
			} else {
				currentLine = word
			}
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
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
