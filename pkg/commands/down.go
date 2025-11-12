package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewDownCommand creates the down command
func NewDownCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop services",
		Long:  `Stop all services (infrastructure keeps running).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			orch := orchestrator.NewOrchestrator(log, cfg)
			return orch.Down(cmd.Context())
		},
	}
}
