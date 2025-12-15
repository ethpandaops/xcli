package commands

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabXatuCBTCommand creates the lab xatu-cbt command namespace.
func NewLabXatuCBTCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "xatu-cbt",
		Short: "Xatu-CBT related commands",
		Long: `Commands for working with xatu-cbt, including generating seed data
for tests.

Common workflows:
  1. Generate seed data for a single external model:
     xcli lab xatu-cbt generate-seed-data

  2. Generate test YAML for transformation models (auto-resolves dependencies):
     xcli lab xatu-cbt generate-transformation-test

Use 'xcli lab xatu-cbt [command] --help' for more information about a command.`,
	}

	// Add xatu-cbt subcommands
	cmd.AddCommand(NewLabXatuCBTGenerateSeedDataCommand(log, configPath))
	cmd.AddCommand(NewLabXatuCBTGenerateTransformationTestCommand(log, configPath))

	return cmd
}
