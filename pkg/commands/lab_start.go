package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
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
			result, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if result.Config.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			orch, err := orchestrator.NewOrchestrator(log, result.Config.Lab, result.ConfigPath)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			if err := orch.StartService(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("failed to start service: %w", err)
			}

			fmt.Printf("\n✓ Service %s started successfully\n", args[0])

			return nil
		},
	}
}
