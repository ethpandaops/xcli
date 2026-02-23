package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuStartCommand creates the xatu start command.
func NewXatuStartCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "start <service>",
		Short: "Start a specific xatu service",
		Long: `Start a specific xatu service that was previously stopped.

Example:
  xcli xatu start xatu-server
  xcli xatu start clickhouse`,
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
			spinner := ui.NewSpinner(fmt.Sprintf("Starting %s", service))

			if err := runner.Start(cmd.Context(), service); err != nil {
				spinner.Fail(fmt.Sprintf("Failed to start %s", service))

				return fmt.Errorf("failed to start service: %w", err)
			}

			spinner.Success(fmt.Sprintf("%s started successfully", service))

			return nil
		},
	}
}
