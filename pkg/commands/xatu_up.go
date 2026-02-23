package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuUpCommand creates the xatu up command.
func NewXatuUpCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var build bool

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start the xatu docker-compose stack",
		Long: `Start the xatu docker-compose stack.

This runs 'docker compose up -d' in the xatu repository directory,
using any configured profiles and environment overrides.

Flags:
  --build   Build images before starting containers

Examples:
  xcli xatu up              # Start all services
  xcli xatu up --build      # Build images and start`,
		RunE: func(cmd *cobra.Command, args []string) error {
			xatuCfg, _, err := config.LoadXatuConfig(configPath)
			if err != nil {
				return err
			}

			if validationErr := xatuCfg.Validate(); validationErr != nil {
				return fmt.Errorf("invalid xatu configuration: %w", validationErr)
			}

			runner, err := newXatuRunner(log, xatuCfg)
			if err != nil {
				return fmt.Errorf("failed to create compose runner: %w", err)
			}

			ui.Header("Starting xatu stack...")
			ui.Blank()

			if err := runner.Up(cmd.Context(), build); err != nil {
				ui.Blank()
				ui.Error("Failed to start xatu stack")

				return err
			}

			ui.Blank()
			ui.Success("Xatu stack started")
			ui.Blank()

			return printXatuStatus(cmd.Context(), runner)
		},
	}

	cmd.Flags().BoolVar(&build, "build", false, "Build images before starting containers")

	return cmd
}
