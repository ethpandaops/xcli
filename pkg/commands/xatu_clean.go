package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuCleanCommand creates the xatu clean command.
func NewXatuCleanCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Remove all xatu containers, volumes, and images",
		Long: `Completely clean the xatu docker-compose stack.

This will:
  - Stop and remove all containers
  - Remove all named volumes (data will be lost!)
  - Remove all images built by the stack

Warning: This is a destructive operation!
  - All data in ClickHouse, Kafka, etc. will be lost
  - All locally built images will be removed
  - You will need to rebuild with 'xcli xatu up --build'

This does NOT remove:
  - Source code or the xatu repository
  - Your .xcli.yaml configuration file

Examples:
  xcli xatu clean`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runXatuClean(cmd, log, configPath)
		},
	}
}

func runXatuClean(cmd *cobra.Command, log logrus.FieldLogger, configPath string) error {
	xatuCfg, _, err := config.LoadXatuConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Confirmation prompt
	ui.Warning("WARNING: This will remove all xatu containers, volumes, and images!")
	fmt.Println("\nThis includes:")
	fmt.Println("  - All Docker containers and volumes (data will be lost)")
	fmt.Println("  - All locally built images")
	fmt.Print("\nContinue? (y/N): ")

	var response string

	_, _ = fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		ui.Info("Cancelled.")

		return nil
	}

	runner, err := newXatuRunner(log, xatuCfg)
	if err != nil {
		return fmt.Errorf("failed to create compose runner: %w", err)
	}

	ui.Header("Removing all xatu containers, volumes, and images...")
	ui.Blank()

	if err := runner.Down(cmd.Context(), true, true); err != nil {
		ui.Blank()
		ui.Error("Failed to clean xatu stack")

		return err
	}

	ui.Blank()
	ui.Success("Xatu workspace cleaned successfully!")

	ui.Header("Next step:")
	fmt.Println("  xcli xatu up --build     # Rebuild and start the stack")

	return nil
}
