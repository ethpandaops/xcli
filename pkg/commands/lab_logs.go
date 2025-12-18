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
		Use:               "logs [service]",
		Short:             "Show lab service logs",
		Long:              `Show logs for all lab services or a specific service.`,
		ValidArgsFunction: completeServices(configPath),
		RunE: func(cmd *cobra.Command, args []string) error {
			labCfg, cfgPath, err := config.LoadLabConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			service := ""
			if len(args) > 0 {
				service = args[0]
			}

			orch, err := orchestrator.NewOrchestrator(log, labCfg, cfgPath)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			return orch.Logs(cmd.Context(), service, follow)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")

	return cmd
}
