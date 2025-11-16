package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabStartCommand creates the lab start command.
func NewLabStartCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "start <service>",
		Short: "Start a specific lab service",
		Long: `Start a specific lab service.

Available services:
  - lab-backend
  - lab-frontend
  - cbt-<network>        (e.g., cbt-mainnet, cbt-sepolia)
  - cbt-api-<network>    (e.g., cbt-api-mainnet, cbt-api-sepolia)

Example:
  xcli lab start lab-backend
  xcli lab start cbt-mainnet`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labCfg, cfgPath, err := config.LoadLabConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			orch, err := orchestrator.NewOrchestrator(log, labCfg, cfgPath)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			service := args[0]

			// Validate service name and provide helpful error
			if !orch.IsValidService(service) {
				ui.Error(fmt.Sprintf("Unknown service: %s", service))
				fmt.Println("\nAvailable services:")

				for _, s := range orch.GetValidServices() {
					fmt.Printf("  - %s\n", s)
				}

				return fmt.Errorf("unknown service: %s", service)
			}

			spinner := ui.NewSpinner(fmt.Sprintf("Starting %s", service))

			if err := orch.StartService(cmd.Context(), service); err != nil {
				spinner.Fail(fmt.Sprintf("Failed to start %s", service))

				return fmt.Errorf("failed to start service: %w", err)
			}

			spinner.Success(fmt.Sprintf("%s started successfully", service))

			return nil
		},
	}
}
