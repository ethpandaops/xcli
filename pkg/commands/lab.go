package commands

import (
	"github.com/ethpandaops/xcli/pkg/stack"
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

	s := stack.NewLabStack(log, configPath)

	// Shared stack commands via factories
	cmd.AddCommand(stack.NewInitCommand(s))
	cmd.AddCommand(stack.NewCheckCommand(s))
	cmd.AddCommand(stack.NewUpCommand(s))
	cmd.AddCommand(stack.NewDownCommand(s))
	cmd.AddCommand(stack.NewCleanCommand(s))
	cmd.AddCommand(stack.NewBuildCommand(s))
	cmd.AddCommand(stack.NewRebuildCommand(s))
	cmd.AddCommand(stack.NewStatusCommand(s))
	cmd.AddCommand(stack.NewLogsCommand(s))
	cmd.AddCommand(stack.NewStartCommand(s))
	cmd.AddCommand(stack.NewStopCommand(s))
	cmd.AddCommand(stack.NewRestartCommand(s))

	// Lab-only commands
	cmd.AddCommand(NewLabModeCommand(log, configPath))
	cmd.AddCommand(NewLabConfigCommand(log, configPath))
	cmd.AddCommand(NewLabOverridesCommand(configPath))
	cmd.AddCommand(NewLabTUICommand(log, configPath))
	cmd.AddCommand(NewLabDiagnoseCommand(log, configPath))
	cmd.AddCommand(NewLabReleaseCommand(log, configPath))
	cmd.AddCommand(NewLabXatuCBTCommand(log, configPath))

	return cmd
}
