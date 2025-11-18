package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Update handles all events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

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
			oldRunning := make(map[string]bool)

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

		// Keep only last 1000 lines per service
		if len(m.logs[line.Service]) > 1000 {
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
		// Scroll logs up - disable follow mode
		m.followMode = false

		m.logScroll -= 10
		if m.logScroll < 0 {
			m.logScroll = 0
		}

	case "pgdown", "d":
		// Scroll logs down - disable follow mode
		m.followMode = false
		m.logScroll += 10

	case "g":
		// Go to bottom and enable follow mode
		m.followMode = true
		m.logScroll = 0
	}

	return m, nil
}

func (m Model) handleMenuKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
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
