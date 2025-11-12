package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabStopCommand creates the lab stop command.
func NewLabStopCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "stop <service>",
		Short: "Stop a specific lab service",
		Long: `Stop a specific lab service.

Available services:
  - lab-backend
  - lab-frontend
  - cbt-<network>        (e.g., cbt-mainnet, cbt-sepolia)
  - cbt-api-<network>    (e.g., cbt-api-mainnet, cbt-api-sepolia)

Example:
  xcli lab stop lab-backend
  xcli lab stop cbt-mainnet`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			orch := orchestrator.NewOrchestrator(log, cfg.Lab)

			if err := orch.StopService(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("failed to stop service: %w", err)
			}

			fmt.Printf("\n✓ Service %s stopped successfully\n", args[0])

			return nil
		},
	}
}
