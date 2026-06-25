package commands

import (
	"github.com/ethpandaops/xcli/pkg/stack"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabCommand creates the lab command namespace.
func NewLabCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var instanceOverride string

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

  4. Stop safely:
     xcli lab down           # Stop services and containers, preserve data
     xcli lab destroy --instance <id>  # Explicit destructive removal

Use 'xcli lab [command] --help' for more information about a command.`,
	}

	cmd.PersistentFlags().StringVar(&instanceOverride, "instance", "", "Lab instance id override")

	s := stack.NewLabStack(log, configPath, &instanceOverride)

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
	cmd.AddCommand(NewLabListCommand())
	cmd.AddCommand(NewLabShowCommand())

	var destroyYes bool

	destroyCmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroy one lab instance and delete its data",
		Long: `Destroy one xcli lab instance and delete its generated state and data volumes.

This is destructive and requires --instance <id>. Without --yes, you must type
the instance id to confirm.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.Destroy(cmd.Context(), destroyYes)
		},
	}
	destroyCmd.Flags().BoolVar(&destroyYes, "yes", false, "Skip confirmation prompt")
	cmd.AddCommand(destroyCmd)

	resetCmd := &cobra.Command{
		Use:   "reset <redis>",
		Short: "Reset one data store for one lab instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.Reset(cmd.Context(), args[0])
		},
	}
	cmd.AddCommand(resetCmd)

	// Lab-only commands
	cmd.AddCommand(NewLabModeCommand(log, configPath, &instanceOverride))
	cmd.AddCommand(NewLabConfigCommand(log, configPath, &instanceOverride))
	cmd.AddCommand(NewLabOverridesCommand(configPath))
	cmd.AddCommand(NewLabTUICommand(log, configPath, &instanceOverride))
	cmd.AddCommand(NewLabDiagnoseCommand(log, configPath, &instanceOverride))
	cmd.AddCommand(NewLabReleaseCommand(log, configPath))
	cmd.AddCommand(NewLabXatuCBTCommand(log, configPath))

	return cmd
}
