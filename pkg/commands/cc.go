package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/cc"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewCCCommand creates the Command Center web dashboard command.
func NewCCCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var (
		port   int
		noOpen bool
	)

	cmd := &cobra.Command{
		Use:   "cc",
		Short: "Launch the Command Center web dashboard",
		Long: `Launch a local web dashboard for monitoring and controlling the lab stack.

The Command Center provides:
  • Real-time service status and health monitoring
  • Live log streaming from all services
  • Interactive service controls (start/stop/restart/rebuild)
  • Infrastructure and git status overview
  • Configuration viewer

The dashboard opens automatically in your default browser.
Use --no-open to prevent this behavior.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			srv, err := cc.NewServer(log, result.Config, result.ConfigPath, port)
			if err != nil {
				return fmt.Errorf("failed to create server: %w", err)
			}

			return srv.Start(cmd.Context(), !noOpen)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 19280, "Port for the web dashboard")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Don't open browser automatically")

	return cmd
}
