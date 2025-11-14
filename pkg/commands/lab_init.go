package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/discovery"
	"github.com/ethpandaops/xcli/pkg/prerequisites"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabInitCommand creates the lab init command.
func NewLabInitCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize lab stack configuration",
		Long: `Initialize lab stack by discovering repositories and configuring the lab section.

This command will:
1. Scan the parent directory for required lab repositories (cbt, xatu-cbt, cbt-api, lab-backend, lab)
2. Offer to clone any missing repositories from GitHub
3. Validate that each repository has the expected structure
4. Update the lab section in .xcli.yaml (creates file if it doesn't exist)

If you haven't run 'xcli init' yet, this command will create the config file automatically.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLabInit(cmd.Context(), log, configPath)
		},
	}
}

func runLabInit(ctx context.Context, log logrus.FieldLogger, configPath string) error {
	log.Info("initializing lab stack")

	// Load existing config if it exists, otherwise start fresh
	var rootCfg *config.Config

	if _, err := os.Stat(configPath); err == nil {
		log.Info("loading existing configuration")

		rootCfg, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("failed to load existing config: %w", err)
		}
	} else {
		rootCfg = &config.Config{}
	}

	// Check if lab config already exists
	if rootCfg.Lab != nil {
		log.Warn("lab configuration already exists")
		fmt.Print("Overwrite existing lab configuration? (y/N): ")

		var response string

		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			log.Info("lab initialization cancelled")

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

	// Run prerequisites for each discovered repository
	prereqChecker := prerequisites.NewChecker(log)

	repoMap := map[string]string{
		"cbt":         repos.CBT,
		"xatu-cbt":    repos.XatuCBT,
		"cbt-api":     repos.CBTAPI,
		"lab-backend": repos.LabBackend,
		"lab":         repos.Lab,
	}

	for repoName, repoPath := range repoMap {
		if err := prereqChecker.Run(ctx, repoPath, repoName); err != nil {
			return fmt.Errorf("failed to run prerequisites for %s: %w", repoName, err)
		}
	}

	// Create lab config with discovered repos
	labCfg := config.DefaultLab()
	labCfg.Repos = *repos

	// Update root config
	rootCfg.Lab = labCfg

	// Save configuration
	if err := rootCfg.Save(configPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	log.WithField("file", configPath).Info("lab configuration updated")

	// Print summary
	fmt.Println("\n✓ Lab stack initialization complete!")
	fmt.Printf("\nDiscovered repositories:\n")
	fmt.Printf("  cbt:         %s\n", repos.CBT)
	fmt.Printf("  xatu-cbt:    %s\n", repos.XatuCBT)
	fmt.Printf("  cbt-api:     %s\n", repos.CBTAPI)
	fmt.Printf("  lab-backend: %s\n", repos.LabBackend)
	fmt.Printf("  lab:         %s\n", repos.Lab)

	fmt.Printf("\nLab configuration saved to: %s\n", configPath)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. Review and edit the 'lab:' section in %s if needed\n", configPath)
	fmt.Printf("  2. Run 'xcli lab up' to start the lab stack\n\n")

	return nil
}
