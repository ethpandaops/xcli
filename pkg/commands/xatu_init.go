package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/ethpandaops/xcli/pkg/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuInitCommand creates the xatu init command.
func NewXatuInitCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the xatu stack environment",
		Long: `Initialize the xatu stack environment by discovering the xatu repository,
verifying Docker and Docker Compose are available, and saving configuration.

This command will:
  - Search for the xatu repo at ../xatu (relative to config file)
  - Verify docker-compose.yml exists in the repo
  - Check Docker and Docker Compose are available
  - Save xatu configuration to .xcli.yaml

After 'xcli xatu init' succeeds, you can start the stack with 'xcli xatu up'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runXatuInit(cmd.Context(), log, configPath)
		},
	}
}

func runXatuInit(ctx context.Context, log logrus.FieldLogger, configPath string) error {
	ui.PrintInitBanner(version.GetVersion())

	log.Info("initializing xatu stack")

	// Load existing config if it exists, otherwise start fresh
	var (
		rootCfg            *config.Config
		resolvedConfigPath string
	)

	if _, err := os.Stat(configPath); err == nil {
		log.Info("loading existing configuration")

		result, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("failed to load existing config: %w", err)
		}

		rootCfg = result.Config
		resolvedConfigPath = result.ConfigPath
	} else {
		rootCfg = &config.Config{}

		absPath, err := filepath.Abs(configPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute config path: %w", err)
		}

		resolvedConfigPath = absPath
	}

	// Check if xatu config already exists
	if rootCfg.Xatu != nil {
		log.Warn("xatu configuration already exists")
		fmt.Print("Overwrite existing xatu configuration? (y/N): ")

		var response string

		_, _ = fmt.Scanln(&response)

		if response != "y" && response != "Y" {
			log.Info("xatu initialization cancelled")

			return nil
		}
	}

	// Discover xatu repo
	ui.Header("Discovering xatu repository")

	configDir := filepath.Dir(resolvedConfigPath)
	xatuPath := filepath.Join(configDir, "..", "xatu")

	absXatuPath, err := filepath.Abs(xatuPath)
	if err != nil {
		return fmt.Errorf("failed to resolve xatu path: %w", err)
	}

	spinner := ui.NewSpinner("Looking for xatu repository")

	if _, statErr := os.Stat(absXatuPath); os.IsNotExist(statErr) {
		spinner.Warning("Xatu repo not found at " + absXatuPath)

		fmt.Print("\nClone xatu from github.com/ethpandaops/xatu? (y/N): ")

		var response string

		_, _ = fmt.Scanln(&response)

		if response != "y" && response != "Y" {
			return fmt.Errorf("xatu repository is required - clone it manually or run init again")
		}

		cloneSpinner := ui.NewSpinner("Cloning xatu repository")

		parentDir := filepath.Dir(absXatuPath)

		cloneCmd := exec.CommandContext(ctx, "git", "clone",
			"https://github.com/ethpandaops/xatu.git", absXatuPath)
		cloneCmd.Dir = parentDir

		if output, cloneErr := cloneCmd.CombinedOutput(); cloneErr != nil {
			cloneSpinner.Fail("Failed to clone xatu repository")

			return fmt.Errorf("git clone failed: %s: %w", string(output), cloneErr)
		}

		cloneSpinner.Success("Cloned xatu repository to " + absXatuPath)
	} else {
		spinner.Success("Found xatu repository at " + absXatuPath)
	}

	// Verify docker-compose.yml exists
	spinner = ui.NewSpinner("Checking docker-compose.yml")

	composePath := filepath.Join(absXatuPath, "docker-compose.yml")
	if _, statErr := os.Stat(composePath); os.IsNotExist(statErr) {
		spinner.Fail("docker-compose.yml not found in xatu repo")

		return fmt.Errorf("docker-compose.yml not found at: %s", composePath)
	}

	spinner.Success("docker-compose.yml found")

	// Check Docker
	ui.Header("Checking prerequisites")

	spinner = ui.NewSpinner("Checking Docker daemon")

	if dockerErr := exec.CommandContext(ctx, "docker", "info").Run(); dockerErr != nil {
		spinner.Fail("Docker daemon not accessible - Ensure Docker Desktop is running")

		return fmt.Errorf("docker is required but not available: %w", dockerErr)
	}

	spinner.Success("Docker daemon accessible")

	// Check Docker Compose
	spinner = ui.NewSpinner("Checking Docker Compose")

	if composeErr := exec.CommandContext(ctx, "docker", "compose", "version").Run(); composeErr != nil {
		spinner.Fail("Docker Compose not available")

		return fmt.Errorf("docker compose is required but not available: %w", composeErr)
	}

	spinner.Success("Docker Compose available")

	// Create xatu config with absolute path (consistent with lab repos)
	xatuCfg := config.DefaultXatu()
	xatuCfg.Repos.Xatu = absXatuPath

	rootCfg.Xatu = xatuCfg

	// Save configuration
	if err := rootCfg.Save(resolvedConfigPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	log.WithField("file", resolvedConfigPath).Info("xatu configuration updated")

	// Print summary
	ui.Blank()
	ui.Success("Xatu stack initialization complete!")

	ui.Header("Configuration:")

	rows := [][]string{
		{"Repo", absXatuPath},
		{"Compose file", composePath},
	}
	ui.Table([]string{"Setting", "Value"}, rows)

	ui.Blank()
	ui.Info(fmt.Sprintf("Xatu configuration saved to: %s", resolvedConfigPath))

	ui.Header("Next steps:")
	fmt.Println("  1. Review the 'xatu:' section in .xcli.yaml if needed")
	fmt.Println("  2. Run 'xcli xatu check' to verify the environment")
	fmt.Println("  3. Run 'xcli xatu up' to start the xatu stack")

	return nil
}
