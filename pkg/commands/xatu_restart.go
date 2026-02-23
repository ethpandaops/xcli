package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuRestartCommand creates the xatu restart command.
func NewXatuRestartCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "restart <service>",
		Short: "Restart a specific xatu service",
		Long: `Restart a specific xatu service.

Example:
  xcli xatu restart xatu-server
  xcli xatu restart clickhouse`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeXatuServices(configPath),
		RunE: func(cmd *cobra.Command, args []string) error {
			xatuCfg, _, err := config.LoadXatuConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			runner, err := newXatuRunner(log, xatuCfg)
			if err != nil {
				return fmt.Errorf("failed to create compose runner: %w", err)
			}

			service := args[0]
			spinner := ui.NewSpinner(fmt.Sprintf("Restarting %s", service))

			if err := runner.Restart(cmd.Context(), service); err != nil {
				spinner.Fail(fmt.Sprintf("Failed to restart %s", service))

				return fmt.Errorf("failed to restart service: %w", err)
			}

			spinner.Success(fmt.Sprintf("%s restarted successfully", service))

			return nil
		},
	}
}
