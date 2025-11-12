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
		Short: "Stop lab stack and remove data",
		Long:  `Stop all lab services, tear down infrastructure, and remove all volumes (clean slate).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			orch := orchestrator.NewOrchestrator(log, cfg.Lab)

			return orch.Down(cmd.Context())
		},
	}
}
