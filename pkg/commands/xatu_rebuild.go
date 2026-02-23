package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuRebuildCommand creates the xatu rebuild command.
func NewXatuRebuildCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "rebuild <service>",
		Short: "Rebuild and restart a xatu service",
		Long: `Rebuild a service's docker image and restart it.

This builds the image with 'docker compose build <service>' (showing full
build output) then starts it with 'docker compose up -d <service>'.

For source-built services (xatu-server, xatu-cannon, xatu-sentry-logs, etc.)
this rebuilds from the Dockerfile. For image-based services (clickhouse, kafka, etc.)
this recreates the container with the latest image.

Examples:
  xcli xatu rebuild xatu-server         # Rebuild and restart xatu-server
  xcli xatu rebuild xatu-cannon         # Rebuild and restart xatu-cannon`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeXatuServices(configPath),
		RunE: func(cmd *cobra.Command, args []string) error {
			xatuCfg, _, err := config.LoadXatuConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			runner, err := newXatuRunner(log, xatuCfg)
			if err != nil {
				return fmt.Errorf("failed to create compose runner: %w", err)
			}

			service := args[0]

			// Step 1: Build (shows full build output including errors)
			ui.Header(fmt.Sprintf("[1/2] Building %s...", service))
			ui.Blank()

			if err := runner.Build(cmd.Context(), service); err != nil {
				ui.Blank()
				ui.Error(fmt.Sprintf("Failed to build %s", service))

				return fmt.Errorf("failed to build service: %w", err)
			}

			ui.Blank()

			// Step 2: Restart with the new image
			ui.Header(fmt.Sprintf("[2/2] Restarting %s...", service))
			ui.Blank()

			if err := runner.Up(cmd.Context(), false, service); err != nil {
				ui.Blank()
				ui.Error(fmt.Sprintf("Failed to restart %s", service))

				return fmt.Errorf("failed to restart service: %w", err)
			}

			ui.Blank()
			ui.Success(fmt.Sprintf("%s rebuilt and restarted", service))

			return nil
		},
	}
}
