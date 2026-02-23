package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuDownCommand creates the xatu down command.
func NewXatuDownCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var volumes bool

	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop the xatu docker-compose stack",
		Long: `Stop all running services in the xatu docker-compose stack.

This runs 'docker compose down' in the xatu repository directory.

Flags:
  --volumes/-v   Remove named volumes declared in the volumes section

Examples:
  xcli xatu down           # Stop all services
  xcli xatu down -v        # Stop and remove volumes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			xatuCfg, _, err := config.LoadXatuConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			runner, err := newXatuRunner(log, xatuCfg)
			if err != nil {
				return fmt.Errorf("failed to create compose runner: %w", err)
			}

			spinner := ui.NewSpinner("Stopping xatu stack")

			if err := runner.Down(cmd.Context(), volumes, false); err != nil {
				spinner.Fail("Failed to stop xatu stack")

				return err
			}

			spinner.Success("Xatu stack stopped")

			return nil
		},
	}

	cmd.Flags().BoolVarP(&volumes, "volumes", "v", false, "Remove named volumes")

	return cmd
}
