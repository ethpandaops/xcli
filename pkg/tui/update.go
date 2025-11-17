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
	}

	return m, nil
}

func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		// Cycle through panels
		panels := []string{"services", "logs", "infra"}
		for i, p := range panels {
			if p == m.activePanel {
				m.activePanel = panels[(i+1)%len(panels)]

				break
			}
		}

	case "s":
		// Start selected service
		if m.selectedIndex < len(m.services) {
			svc := m.services[m.selectedIndex]
			if svc.Status == statusStopped {
				ctx := context.Background()

				go func() {
					_ = m.wrapper.StartService(ctx, svc.Name) // Errors logged internally
				}()
			}
		}

	case "t":
		// Stop selected service
		if m.selectedIndex < len(m.services) {
			svc := m.services[m.selectedIndex]
			if svc.Status == statusRunning {
				ctx := context.Background()

				go func() {
					_ = m.wrapper.StopService(ctx, svc.Name) // Errors logged internally
				}()
			}
		}

	case "r":
		// Restart selected service
		if m.selectedIndex < len(m.services) {
			svc := m.services[m.selectedIndex]
			ctx := context.Background()

			go func() {
				_ = m.wrapper.RestartService(ctx, svc.Name) // Errors logged internally
			}()
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
