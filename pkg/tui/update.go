package tui

import (
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Update handles all events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.MouseMsg:
		return m.handleMouseEvent(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		return m, nil

	case tickMsg:
		// Refresh service status
		oldServices := m.services
		m.services = m.wrapper.GetServices()
		m.infrastructure = m.wrapper.GetInfrastructure()
		m.lastUpdate = time.Now()

		// Check if any services transitioned to running state
		// and start log streaming for them
		if m.logStreamer != nil {
			oldRunning := make(map[string]bool, len(oldServices))

			for _, svc := range oldServices {
				if svc.Status == statusRunning {
					oldRunning[svc.Name] = true
				}
			}

			for _, svc := range m.services {
				if svc.Status == statusRunning && !oldRunning[svc.Name] && svc.LogFile != "" {
					// Service just started - begin tailing its logs
					_ = m.logStreamer.Start(svc.Name, svc.LogFile) // Non-critical error
				}
			}
		}

		return m, tick()

	case eventMsg:
		// Handle service events
		event := Event(msg)
		m.handleEvent(event)

		return m, waitForEvent(m.eventBus.Subscribe())

	case logMsg:
		// Append log line
		line := LogLine(msg)
		if _, exists := m.logs[line.Service]; !exists {
			m.logs[line.Service] = []LogLine{}
		}

		m.logs[line.Service] = append(m.logs[line.Service], line)

		// Keep only last N lines per service (-1 means unlimited)
		if m.maxLogLines > 0 && len(m.logs[line.Service]) > m.maxLogLines {
			m.logs[line.Service] = m.logs[line.Service][1:]
		}

		return m, nil

	case healthMsg:
		m.health = msg

		return m, nil

	case activityDoneMsg:
		// Activity completed - clear the indicator
		m.activity = ""

		return m, nil
	}

	return m, nil
}

func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle menu-specific keys when menu is open
	if m.showMenu {
		return m.handleMenuKeyPress(msg)
	}

	// Handle log detail mode
	if m.logDetailMode {
		return m.handleLogDetailKeyPress(msg)
	}

	// Handle filter input mode
	if m.filterMode {
		return m.handleFilterKeyPress(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.cleanup()

		return m, tea.Quit

	case "up", "k":
		if m.selectedIndex > 0 {
			m.selectedIndex--
		}

	case "down", "j":
		if m.selectedIndex < len(m.services)-1 {
			m.selectedIndex++
		}

	case "tab":
		// Cycle through panels: services → actions → services
		switch m.activePanel {
		case panelServices:
			m.activePanel = panelActions
		case panelActions:
			m.activePanel = panelServices
		default:
			m.activePanel = panelServices
		}

	case "enter":
		// Behavior depends on active panel
		if m.activePanel == panelActions {
			// Activate Rebuild All
			m.activity = "Rebuilding all services..."
			m.activityStart = time.Now()

			return m, func() tea.Msg {
				ctx := context.Background()
				err := m.wrapper.RebuildAll(ctx)

				return activityDoneMsg{err: err}
			}
		} else if m.selectedIndex < len(m.services) {
			// Open action menu for selected service
			svc := m.services[m.selectedIndex]
			m.menuActions = GetMenuActions(svc.Status)
			m.showMenu = true
		}

	case "a":
		// Open action menu for selected service (alternative key)
		if m.activePanel == panelServices && m.selectedIndex < len(m.services) {
			svc := m.services[m.selectedIndex]
			m.menuActions = GetMenuActions(svc.Status)
			m.showMenu = true
		}

	case "r":
		// Quick rebuild (global shortcut)
		if m.selectedIndex < len(m.services) {
			svc := m.services[m.selectedIndex]
			m.activity = "Rebuilding " + svc.Name + "..."
			m.activityStart = time.Now()

			return m, func() tea.Msg {
				ctx := context.Background()
				err := m.wrapper.RebuildService(ctx, svc.Name)

				return activityDoneMsg{err: err}
			}
		}

	case "R":
		// Rebuild All (Shift+R) - full stack rebuild
		m.activity = "Rebuilding all services..."
		m.activityStart = time.Now()

		return m, func() tea.Msg {
			ctx := context.Background()
			err := m.wrapper.RebuildAll(ctx)

			return activityDoneMsg{err: err}
		}

	case "pgup", "u":
		m = m.scrollLogsUp(10)

	case "pgdown", "d":
		m, _ = m.scrollLogsDown(10)

		return m, nil

	case "g":
		// Go to bottom and enable follow mode
		m.followMode = true
		m.logScroll = 0

	case "e":
		// Enter log detail mode to view full log line
		m.logDetailMode = true
		m.selectedLogIndex = 0

	case "f", "/":
		// Enter filter mode
		m.filterMode = true
		m.filterInput = ""

	case "l":
		// Cycle log level filter: ALL -> ERROR -> WARN -> INFO -> DEBUG -> ALL
		m.logLevelFilter = m.nextLogLevel()

	case keyEsc:
		// Clear filter if active
		if m.filterActive {
			m.filterActive = false
			m.filterRegex = ""
			m.filterCompiled = nil
			m.filterError = nil
		}
	}

	return m, nil
}

func (m Model) handleLogDetailKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	visibleLogs := m.getVisibleLogCount()

	switch msg.String() {
	case keyEsc, "e", "q", "ctrl+c":
		// Exit log detail mode
		m.logDetailMode = false

	case "up", "k":
		// Select previous log line
		if m.selectedLogIndex > 0 {
			m.selectedLogIndex--
		}

	case "down", "j":
		// Select next log line
		if m.selectedLogIndex < visibleLogs-1 {
			m.selectedLogIndex++
		}

	case "c", "y":
		// Copy selected log to clipboard (stripped of ANSI codes)
		if log := m.getSelectedLog(); log != nil {
			_ = copyToClipboard(log.Message)
		}
	}

	return m, nil
}

func (m Model) handleFilterKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		// Exit filter mode without applying
		m.filterMode = false
		m.filterInput = ""

	case tea.KeyEnter:
		// Apply filter
		m.filterMode = false
		if m.filterInput != "" {
			m.filterRegex = m.filterInput
			m.filterActive = true
			// Pre-compile the regex
			compiled, err := regexp.Compile(m.filterInput)
			m.filterCompiled = compiled
			m.filterError = err
		} else {
			// Clear filter if input is empty
			m.filterActive = false
			m.filterRegex = ""
			m.filterCompiled = nil
			m.filterError = nil
		}

		m.filterInput = ""

	case tea.KeyBackspace:
		// Remove last character
		if len(m.filterInput) > 0 {
			m.filterInput = m.filterInput[:len(m.filterInput)-1]
		}

	case tea.KeySpace:
		// Add space
		m.filterInput += " "

	case tea.KeyRunes:
		// Add typed characters
		m.filterInput += string(msg.Runes)
	}

	return m, nil
}

func (m Model) handleMenuKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc, "q":
		// Close menu
		m.showMenu = false
		m.menuActions = nil

	case "s":
		// Start service
		if m.selectedIndex < len(m.services) {
			svc := m.services[m.selectedIndex]
			if svc.Status == statusStopped {
				m.activity = "Starting " + svc.Name + "..."
				m.activityStart = time.Now()
				m.showMenu = false
				m.menuActions = nil

				return m, func() tea.Msg {
					ctx := context.Background()
					err := m.wrapper.StartService(ctx, svc.Name)

					return activityDoneMsg{err: err}
				}
			}
		}

		m.showMenu = false
		m.menuActions = nil

	case "t":
		// Stop service
		if m.selectedIndex < len(m.services) {
			svc := m.services[m.selectedIndex]
			if svc.Status == statusRunning {
				m.activity = "Stopping " + svc.Name + "..."
				m.activityStart = time.Now()
				m.showMenu = false
				m.menuActions = nil

				return m, func() tea.Msg {
					ctx := context.Background()
					err := m.wrapper.StopService(ctx, svc.Name)

					return activityDoneMsg{err: err}
				}
			}
		}

		m.showMenu = false
		m.menuActions = nil

	case "r":
		// Restart service
		if m.selectedIndex < len(m.services) {
			svc := m.services[m.selectedIndex]
			m.activity = "Restarting " + svc.Name + "..."
			m.activityStart = time.Now()
			m.showMenu = false
			m.menuActions = nil

			return m, func() tea.Msg {
				ctx := context.Background()
				err := m.wrapper.RestartService(ctx, svc.Name)

				return activityDoneMsg{err: err}
			}
		}

		m.showMenu = false
		m.menuActions = nil

	case "b":
		// Rebuild service
		if m.selectedIndex < len(m.services) {
			svc := m.services[m.selectedIndex]
			m.activity = "Rebuilding " + svc.Name + "..."
			m.activityStart = time.Now()
			m.showMenu = false
			m.menuActions = nil

			return m, func() tea.Msg {
				ctx := context.Background()
				err := m.wrapper.RebuildService(ctx, svc.Name)

				return activityDoneMsg{err: err}
			}
		}

		m.showMenu = false
		m.menuActions = nil

	case "l":
		// Open logs in new terminal window
		if m.selectedIndex < len(m.services) {
			svc := m.services[m.selectedIndex]
			if svc.Status == statusRunning {
				go openLogsInNewTerminal(svc.Name)
			}
		}

		m.showMenu = false
		m.menuActions = nil
	}

	return m, nil
}

func (m Model) handleEvent(event Event) {
	// Update UI based on event type
	switch event.Type {
	case EventServiceStarted:
		// Start log streaming for newly started service
		if m.logStreamer != nil {
			// Find the service to get its log file
			for _, svc := range m.wrapper.GetServices() {
				if svc.Name == event.Service && svc.LogFile != "" {
					_ = m.logStreamer.Start(svc.Name, svc.LogFile) // Non-critical error

					break
				}
			}
		}
	case EventServiceStopped:
		// No action needed
	case EventServiceCrashed:
		// No action needed - status will be reflected in service list
	}
}

func (m Model) handleMouseEvent(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m = m.scrollLogsUp(3)

	case tea.MouseButtonWheelDown:
		m, _ = m.scrollLogsDown(3)

		return m, nil

	case tea.MouseButtonLeft:
		// Check if click is in the logs panel area
		if logIndex, ok := m.getClickedLogIndex(msg.Y); ok {
			// Pause log scrolling when clicking a log
			if m.followMode {
				m.followMode = false
				m.logScroll = m.getMaxLogScroll()
			}

			m.selectedLogIndex = logIndex
			m.logDetailMode = true
		}
	}

	return m, nil
}

// scrollLogsUp scrolls logs up by the given amount, handling follow mode transition.
// Returns the updated model for Bubbletea.
func (m Model) scrollLogsUp(amount int) Model {
	// If in follow mode, transition to manual scroll at current position
	if m.followMode {
		m.followMode = false
		m.logScroll = m.getMaxLogScroll()
	}

	m.logScroll -= amount

	if m.logScroll < 0 {
		m.logScroll = 0
	}

	return m
}

// scrollLogsDown scrolls logs down by the given amount.
// Returns the updated model and whether scrolling occurred.
func (m Model) scrollLogsDown(amount int) (Model, bool) {
	// If in follow mode, already at bottom - nothing to do
	if m.followMode {
		return m, false
	}

	m.logScroll += amount

	// Clamp to max scroll position
	maxScroll := m.getMaxLogScroll()
	if m.logScroll > maxScroll {
		m.logScroll = maxScroll
	}

	return m, true
}

// getClickedLogIndex returns the log line index if the click is within the logs panel.
func (m Model) getClickedLogIndex(clickY int) (int, bool) {
	// Calculate logs content start position using layout constants
	logsPanelStart := m.getLogsPanelContentStart()

	// Calculate logs panel end (before help and status bar ~4 lines)
	logsPanelEnd := m.height - 4

	// Check if click is within logs content area
	if clickY < logsPanelStart || clickY >= logsPanelEnd {
		return 0, false
	}

	// Calculate which log line was clicked (relative to visible logs)
	clickedIndex := clickY - logsPanelStart

	// Validate against visible log count
	visibleLogs := m.getVisibleLogCount()
	if clickedIndex < 0 || clickedIndex >= visibleLogs {
		return 0, false
	}

	return clickedIndex, true
}

// getLogsPanelContentStart returns the Y position where log content begins.
func (m Model) getLogsPanelContentStart() int {
	// Calculate services panel height
	servicesHeight := len(m.services) + 4
	if servicesHeight > servicesPanelMaxHeight {
		servicesHeight = servicesPanelMaxHeight
	}

	// Layout: Title (~2) + Services panel + borders (2) + Logs border (1) + Header (1) + Separator (1)
	start := 2 + servicesHeight + 2 + 1 + 1 + 1

	// If filter mode is active, add 2 more lines for filter bar
	if m.filterMode {
		start += 2
	}

	return start
}

// getLogPanelHeight returns the calculated height for the log panel.
func (m Model) getLogPanelHeight() int {
	height := m.height - logPanelReservedHeight
	if height < logPanelMinHeight {
		height = logPanelMinHeight
	}

	return height
}

// getVisibleLogCount returns the number of visible log lines in the current view.
func (m Model) getVisibleLogCount() int {
	if m.selectedIndex >= len(m.services) {
		return 0
	}

	selectedService := m.services[m.selectedIndex].Name
	logs := m.logs[selectedService]

	// Apply log level filter
	logs = m.filterLogsByLevel(logs)

	// Apply regex filter if active
	if m.filterActive && m.filterCompiled != nil {
		logs = m.filterLogs(logs)
	}

	visibleLines := m.getLogPanelHeight() - 2 // Account for header and separator

	// Return minimum of total logs and visible lines
	if len(logs) < visibleLines {
		return len(logs)
	}

	return visibleLines
}

// getMaxLogScroll returns the maximum valid scroll position for the current service's logs.
func (m Model) getMaxLogScroll() int {
	if m.selectedIndex >= len(m.services) {
		return 0
	}

	selectedService := m.services[m.selectedIndex].Name
	logs := m.logs[selectedService]

	// Apply log level filter
	logs = m.filterLogsByLevel(logs)

	// Apply regex filter if active
	if m.filterActive && m.filterCompiled != nil {
		logs = m.filterLogs(logs)
	}

	visibleLines := m.getLogPanelHeight() - 2 // Account for header and separator
	maxScroll := len(logs) - visibleLines

	if maxScroll < 0 {
		return 0
	}

	return maxScroll
}

func (m Model) cleanup() {
	if m.updateTicker != nil {
		m.updateTicker.Stop()
	}

	if m.logStreamer != nil {
		m.logStreamer.Stop()
	}

	if m.healthMonitor != nil {
		m.healthMonitor.Stop()
	}

	if m.eventBus != nil {
		m.eventBus.Close()
	}
}

// getSelectedLog returns the currently selected log line, or nil if no valid selection.
func (m Model) getSelectedLog() *LogLine {
	if m.selectedIndex >= len(m.services) {
		return nil
	}

	selectedService := m.services[m.selectedIndex].Name
	logs := m.logs[selectedService]

	// Apply log level filter
	logs = m.filterLogsByLevel(logs)

	// Apply regex filter if active
	if m.filterActive && m.filterCompiled != nil {
		logs = m.filterLogs(logs)
	}

	// Calculate visible window
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
		return nil
	}

	return &logs[logIndex]
}

// nextLogLevel returns the next log level in the cycle: ALL -> ERROR -> WARN -> INFO -> DEBUG -> ALL.
func (m Model) nextLogLevel() string {
	switch m.logLevelFilter {
	case LogLevelAll:
		return LogLevelError
	case LogLevelError:
		return LogLevelWarn
	case LogLevelWarn:
		return LogLevelInfo
	case LogLevelInfo:
		return LogLevelDebug
	case LogLevelDebug:
		return LogLevelAll
	default:
		return LogLevelAll
	}
}

// openLogsInNewTerminal opens a new terminal window with logs for the service.
func openLogsInNewTerminal(serviceName string) {
	cmd := "xcli lab logs -f " + serviceName

	switch runtime.GOOS {
	case "darwin":
		// Open in Terminal.app
		script := `tell application "Terminal"
			do script "` + cmd + `"
			activate
		end tell`
		// #nosec G204 - serviceName is from internal service list, not user input
		_ = exec.Command("osascript", "-e", script).Start()

	case "linux":
		// Try common terminal emulators in order of preference
		if _, err := exec.LookPath("gnome-terminal"); err == nil {
			// #nosec G204 - cmd is constructed from internal service name
			_ = exec.Command("gnome-terminal", "--", "sh", "-c", cmd).Start()
		} else if _, err := exec.LookPath("konsole"); err == nil {
			// #nosec G204 - cmd is constructed from internal service name
			_ = exec.Command("konsole", "-e", "sh", "-c", cmd).Start()
		} else if _, err := exec.LookPath("xterm"); err == nil {
			// #nosec G204 - cmd is constructed from internal service name
			_ = exec.Command("xterm", "-e", cmd).Start()
		}
	}
}
