package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewStatusCommand creates the status command
func NewStatusCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show stack health status",
		Long:  `Show health status of infrastructure and services.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			orch := orchestrator.NewOrchestrator(log, cfg)
			return orch.Status(cmd.Context())
		},
	}
}
