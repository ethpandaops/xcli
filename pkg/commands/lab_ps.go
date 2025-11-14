package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabPsCommand creates the lab ps command.
func NewLabPsCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List running lab services",
		Long:  `List all running lab services and their status.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			orch, err := orchestrator.NewOrchestrator(log, cfg.Lab)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			return orch.Status(cmd.Context())
		},
	}
}
