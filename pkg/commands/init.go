package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/discovery"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const configFileName = ".xcli.yaml"

// NewInitCommand creates the init command
func NewInitCommand(log logrus.FieldLogger) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize xcli configuration",
		Long: `Initialize xcli by discovering repositories and creating a configuration file.

This command will:
1. Scan the parent directory for required repositories (cbt, xatu-cbt, cbt-api, lab-backend, lab)
2. Validate that each repository has the expected structure
3. Create a .xcli.yaml configuration file with discovered paths`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd.Context(), log)
		},
	}
}

func runInit(ctx context.Context, log logrus.FieldLogger) error {
	log.Info("initializing xcli")

	// Check if config already exists
	if _, err := os.Stat(configFileName); err == nil {
		log.Warn("configuration file already exists")
		fmt.Print("Overwrite existing configuration? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			log.Info("Initialization cancelled")
			return nil
		}
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Discover repositories in parent directory
	parentDir := filepath.Join(cwd, "..")
	disc := discovery.NewDiscovery(log, parentDir)

	repos, err := disc.DiscoverRepos()
	if err != nil {
		return fmt.Errorf("repository discovery failed: %w", err)
	}

	// Create default config with discovered repos
	cfg := config.Default()
	cfg.Repos = *repos

	// Save configuration
	if err := cfg.Save(configFileName); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	log.WithField("file", configFileName).Info("configuration file created")

	// Print summary
	fmt.Println("\n✓ Initialization complete!")
	fmt.Printf("\nDiscovered repositories:\n")
	fmt.Printf("  cbt:         %s\n", repos.CBT)
	fmt.Printf("  xatu-cbt:    %s\n", repos.XatuCBT)
	fmt.Printf("  cbt-api:     %s\n", repos.CBTAPI)
	fmt.Printf("  lab-backend: %s\n", repos.LabBackend)
	fmt.Printf("  lab:         %s\n", repos.Lab)

	fmt.Printf("\nConfiguration saved to: %s\n", configFileName)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. Review and edit %s if needed\n", configFileName)
	fmt.Printf("  2. Run 'xcli up' to start the stack\n\n")

	return nil
}
