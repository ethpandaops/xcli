package commands

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabCommand creates the lab command namespace.
func NewLabCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lab",
		Short: "Manage the lab stack",
		Long:  `Commands for managing the lab development stack and services.`,
	}

	// Add lab subcommands
	cmd.AddCommand(NewLabInitCommand(log, configPath))
	cmd.AddCommand(NewLabUpCommand(log, configPath))
	cmd.AddCommand(NewLabDownCommand(log, configPath))
	cmd.AddCommand(NewLabBuildCommand(log, configPath))
	cmd.AddCommand(NewLabPsCommand(log, configPath))
	cmd.AddCommand(NewLabLogsCommand(log, configPath))
	cmd.AddCommand(NewLabStartCommand(log, configPath))
	cmd.AddCommand(NewLabStopCommand(log, configPath))
	cmd.AddCommand(NewLabRestartCommand(log, configPath))
	cmd.AddCommand(NewLabModeCommand(log, configPath))
	cmd.AddCommand(NewLabConfigCommand(log, configPath))

	return cmd
}
