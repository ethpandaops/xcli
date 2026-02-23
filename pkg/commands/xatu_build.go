package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuBuildCommand creates the xatu build command.
func NewXatuBuildCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "build [service...]",
		Short: "Build xatu docker images",
		Long: `Build docker images for xatu services without starting them.

If no service is specified, all services with build configurations are built.

Examples:
  xcli xatu build                   # Build all images
  xcli xatu build xatu-server       # Build just xatu-server image`,
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

			ui.Header("Building xatu images...")
			ui.Blank()

			if err := runner.Build(cmd.Context(), args...); err != nil {
				ui.Blank()
				ui.Error("Build failed")

				return err
			}

			ui.Blank()
			ui.Success("Build complete")

			return nil
		},
	}
}
