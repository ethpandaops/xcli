package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabLogsCommand creates the lab logs command.
func NewLabLogsCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "Show lab service logs",
		Long:  `Show logs for all lab services or a specific service.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			service := ""
			if len(args) > 0 {
				service = args[0]
			}

			orch := orchestrator.NewOrchestrator(log, cfg.Lab)

			return orch.Logs(cmd.Context(), service, follow)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")

	return cmd
}
