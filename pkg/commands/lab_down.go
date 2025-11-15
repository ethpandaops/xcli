package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabDownCommand creates the lab down command.
func NewLabDownCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop the xcli lab stack",
		Long: `Stop all running services and infrastructure in the xcli lab stack.

This will:
  - Stop all application services (CBT, cbt-api, lab-backend, frontend)
  - Stop infrastructure (ClickHouse, Redis)
  - Remove Docker containers and volumes

The stack can be restarted with 'xcli lab up'.

Example:
  xcli lab down`,
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

			return orch.Down(cmd.Context())
		},
	}
}
