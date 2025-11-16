package commands

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabCommand creates the lab command namespace.
func NewLabCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lab",
		Short: "Manage the xcli lab environment",
		Long: `Manage the complete xcli lab environment including initialization,
infrastructure, builds, and services.

The lab operates in two modes:
  - local: Fully local stack with no external dependencies
  - hybrid: Uses external ClickHouse database with local processing

Common workflows:
  1. Initial setup:
     xcli lab init           # Clone repos, check prerequisites
     xcli lab check          # Verify environment is ready
     xcli lab up             # Start the stack

  2. Local development:
     (make code changes)
     xcli lab rebuild cbt-api   # Rebuild specific component
     xcli lab status            # Check service status

  3. Mode switching:
     xcli lab mode hybrid    # Switch to hybrid mode
     xcli lab up             # Restart in new mode

  4. Clean workspace:
     xcli lab clean          # Remove all containers and artifacts

Use 'xcli lab [command] --help' for more information about a command.`,
	}

	// Add lab subcommands
	cmd.AddCommand(NewLabInitCommand(log, configPath))
	cmd.AddCommand(NewLabCheckCommand(log, configPath))
	cmd.AddCommand(NewLabUpCommand(log, configPath))
	cmd.AddCommand(NewLabDownCommand(log, configPath))
	cmd.AddCommand(NewLabCleanCommand(log, configPath))
	cmd.AddCommand(NewLabBuildCommand(log, configPath))
	cmd.AddCommand(NewLabRebuildCommand(log, configPath))
	cmd.AddCommand(NewLabStatusCommand(log, configPath))
	cmd.AddCommand(NewLabLogsCommand(log, configPath))
	cmd.AddCommand(NewLabStartCommand(log, configPath))
	cmd.AddCommand(NewLabStopCommand(log, configPath))
	cmd.AddCommand(NewLabRestartCommand(log, configPath))
	cmd.AddCommand(NewLabModeCommand(log, configPath))
	cmd.AddCommand(NewLabConfigCommand(log, configPath))

	return cmd
}
