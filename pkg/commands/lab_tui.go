package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/tui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabTUICommand creates the lab TUI command.
func NewLabTUICommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch interactive TUI dashboard",
		Long: `Launch an interactive terminal dashboard for monitoring and controlling the lab stack.

The dashboard provides:
  • Real-time service status monitoring
  • Live log streaming from all services
  • Interactive service controls (start/stop/restart)
  • Infrastructure health monitoring
  • Keyboard shortcuts for quick operations

Keyboard shortcuts:
  • ↑/↓ or j/k: Navigate services
  • s: Start selected service
  • t: Stop selected service
  • r: Restart selected service
  • Tab: Switch between panels
  • PgUp/PgDown: Scroll logs
  • q or Ctrl+C: Quit

Note: TUI requires an interactive terminal (TTY).
For non-interactive use, use 'xcli lab status' instead.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config
			labCfg, cfgPath, err := config.LoadLabConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Initialize orchestrator
			orch, err := orchestrator.NewOrchestrator(log, labCfg, cfgPath)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			// Run TUI
			return tui.Run(orch, labCfg.TUI.MaxLogLines)
		},
	}
}
