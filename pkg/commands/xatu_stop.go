package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuStopCommand creates the xatu stop command.
func NewXatuStopCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "stop <service>",
		Short: "Stop a specific xatu service",
		Long: `Stop a specific xatu service.

Example:
  xcli xatu stop xatu-server
  xcli xatu stop clickhouse`,
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
			spinner := ui.NewSpinner(fmt.Sprintf("Stopping %s", service))

			if err := runner.Stop(cmd.Context(), service); err != nil {
				spinner.Fail(fmt.Sprintf("Failed to stop %s", service))

				return fmt.Errorf("failed to stop service: %w", err)
			}

			spinner.Success(fmt.Sprintf("%s stopped successfully", service))

			return nil
		},
	}
}
