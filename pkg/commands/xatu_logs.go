package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuLogsCommand creates the xatu logs command.
func NewXatuLogsCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "Show xatu service logs",
		Long: `Show logs for all xatu services or a specific service.

Examples:
  xcli xatu logs                    # Show logs for all services
  xcli xatu logs xatu-server        # Show logs for xatu-server
  xcli xatu logs xatu-server -f     # Follow xatu-server logs`,
		ValidArgsFunction: completeXatuServices(configPath),
		RunE: func(cmd *cobra.Command, args []string) error {
			xatuCfg, _, err := config.LoadXatuConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			service := ""
			if len(args) > 0 {
				service = args[0]
			}

			runner, err := newXatuRunner(log, xatuCfg)
			if err != nil {
				return fmt.Errorf("failed to create compose runner: %w", err)
			}

			return runner.Logs(cmd.Context(), service, follow)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")

	return cmd
}
