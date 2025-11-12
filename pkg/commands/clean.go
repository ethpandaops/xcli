package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewCleanCommand creates the clean command
func NewCleanCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Stop everything and remove data",
		Long:  `Stop all services and infrastructure, remove all data (clean slate).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			orch := orchestrator.NewOrchestrator(log, cfg)
			return orch.Clean(cmd.Context())
		},
	}
}
