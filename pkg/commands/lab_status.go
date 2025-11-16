package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabStatusCommand creates the lab status command.
func NewLabStatusCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show lab stack status",
		Long: `Display status of all lab services and infrastructure.

Shows:
  • Running services and their states
  • Port bindings
  • Container health
  • Infrastructure status (ClickHouse, Redis)

Example:
  xcli lab status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			labCfg, cfgPath, err := config.LoadLabConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			orch, err := orchestrator.NewOrchestrator(log, labCfg, cfgPath)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			return orch.Status(cmd.Context())
		},
	}
}
