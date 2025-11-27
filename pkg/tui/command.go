package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
)

// Run starts the TUI dashboard.
func Run(orch *orchestrator.Orchestrator, maxLogLines int) error {
	// Check if running in TTY
	if !isatty() {
		return fmt.Errorf("TUI requires an interactive terminal\nUse 'xcli lab status' for non-interactive environments")
	}

	// Create wrapper
	wrapper := NewOrchestratorWrapper(orch)

	// Initialize model
	model := NewModel(wrapper, maxLogLines)

	// Start log streaming
	logStreamer := NewLogStreamer()

	for _, svc := range wrapper.GetServices() {
		if svc.Status == statusRunning && svc.LogFile != "" {
			_ = logStreamer.Start(svc.Name, svc.LogFile) // Error is non-critical
		}
	}

	model.logStreamer = logStreamer

	// Start health monitoring
	healthMonitor := NewHealthMonitor(wrapper)
	healthMonitor.Start()
	model.healthMonitor = healthMonitor

	// Run TUI
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Start goroutine to forward log messages to Bubbletea
	// This reads from the shared channel that all services write to
	go func() {
		for line := range logStreamer.Output() {
			p.Send(logMsg(line))
		}
	}()

	// Start goroutine to forward health messages to Bubbletea
	go func() {
		for health := range healthMonitor.Output() {
			p.Send(healthMsg(health))
		}
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

func isatty() bool {
	fileInfo, _ := os.Stdout.Stat()

	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
