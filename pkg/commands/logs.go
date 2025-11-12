package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLogsCommand creates the logs command
func NewLogsCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "Show service logs",
		Long:  `Show logs for all services or a specific service.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			service := ""
			if len(args) > 0 {
				service = args[0]
			}

			orch := orchestrator.NewOrchestrator(log, cfg)
			return orch.Logs(cmd.Context(), service, follow)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")

	return cmd
}
