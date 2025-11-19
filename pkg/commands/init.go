package commands

import (
	"fmt"
	"os"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/ethpandaops/xcli/pkg/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewInitCommand creates the root init command.
func NewInitCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize xcli configuration",
		Long: `Initialize xcli by creating a .xcli.yaml configuration file.

This command creates an empty configuration file that you can then populate
by running stack-specific init commands:
  - xcli lab init      (initialize lab stack configuration)
  - xcli <stack> init  (initialize other stack configurations)

If the configuration file already exists, this command will exit without changes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRootInit(log, configPath)
		},
	}
}

func runRootInit(log logrus.FieldLogger, configPath string) error {
	// Print the welcome banner
	ui.PrintInitBanner(version.GetVersion())

	log.Info("initializing xcli configuration")

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		ui.Success(fmt.Sprintf("Configuration file already exists: %s", configPath))
		ui.Blank()
		ui.Info("To initialize specific stacks, run:")
		fmt.Println("  xcli lab init        - Initialize lab stack")
		fmt.Println("  xcli <stack> init    - Initialize other stacks")

		return nil
	}

	// Create empty config
	cfg := &config.Config{}

	// Save empty configuration
	if err := cfg.Save(configPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	log.WithField("file", configPath).Info("configuration file created")

	// Print summary
	ui.Blank()
	ui.Success("xcli configuration initialized!")
	ui.Info(fmt.Sprintf("Configuration file created: %s", configPath))
	ui.Blank()
	ui.Header("Next steps:")
	fmt.Println("  1. Run 'xcli lab init' to discover and configure lab repositories")
	fmt.Println("  2. Run 'xcli <stack> init' for other stacks as needed")
	fmt.Printf("  3. Edit %s to customize settings\n", configPath)

	return nil
}
