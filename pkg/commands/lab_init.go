package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/discovery"
	"github.com/ethpandaops/xcli/pkg/prerequisites"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/ethpandaops/xcli/pkg/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabInitCommand creates the lab init command.
func NewLabInitCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the xcli lab environment",
		Long: `Initialize the xcli lab environment by discovering repositories,
checking prerequisites, and ensuring everything is ready to start.

This command should be run once after installation, or when setting up
a new machine. It will:
  - Discover required repositories (lab, xatu-cbt, cbt-api, lab-backend, cbt)
  - Clone missing repositories
  - Check and install prerequisites (dependencies, node_modules, etc.)
  - Validate configuration

After 'xcli lab init' succeeds, you can start the stack with 'xcli lab up'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLabInit(cmd.Context(), log, configPath)
		},
	}
}

func runLabInit(ctx context.Context, log logrus.FieldLogger, configPath string) error {
	// Print the welcome banner
	ui.PrintInitBanner(version.GetVersion())

	log.Info("initializing lab stack")

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
		// Make config path absolute
		absPath, err := filepath.Abs(configPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute config path: %w", err)
		}

		resolvedConfigPath = absPath
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

	ui.Header("Discovering repositories")

	spinner := ui.NewSpinner("Scanning parent directory for required repositories")

	repos, err := disc.DiscoverRepos()
	if err != nil {
		spinner.Fail("Repository discovery failed")

		return fmt.Errorf("repository discovery failed: %w", err)
	}

	spinner.Success("Found all 5 repositories")

	// Run prerequisites for each discovered repository
	prereqChecker := prerequisites.NewChecker(log)

	repoMap := map[string]string{
		"cbt":         repos.CBT,
		"xatu-cbt":    repos.XatuCBT,
		"cbt-api":     repos.CBTAPI,
		"lab-backend": repos.LabBackend,
		"lab":         repos.Lab,
	}

	ui.Header("Checking prerequisites")

	for repoName, repoPath := range repoMap {
		spinner = ui.NewSpinner(fmt.Sprintf("Running prerequisites for %s", repoName))

		if err := prereqChecker.Run(ctx, repoPath, repoName); err != nil {
			spinner.Fail(fmt.Sprintf("Prerequisites failed for %s", repoName))

			return fmt.Errorf("failed to run prerequisites for %s: %w", repoName, err)
		}

		spinner.Success(fmt.Sprintf("%s prerequisites complete", repoName))
	}

	// Create lab config with discovered repos
	labCfg := config.DefaultLab()
	labCfg.Repos = *repos

	// Update root config
	rootCfg.Lab = labCfg

	// Save configuration
	if err := rootCfg.Save(resolvedConfigPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	log.WithField("file", resolvedConfigPath).Info("lab configuration updated")

	// Register xcli path globally for use from anywhere
	xcliDir := filepath.Dir(resolvedConfigPath)
	if err := config.SetXCLIPath(xcliDir); err != nil {
		log.WithError(err).Warn("failed to register xcli path globally (non-fatal)")
	} else {
		log.WithField("path", xcliDir).Info("registered xcli installation globally")
	}

	// Print summary
	ui.Success("Lab stack initialization complete!")

	ui.Header("Discovered repositories:")

	rows := [][]string{
		{"cbt", repos.CBT},
		{"xatu-cbt", repos.XatuCBT},
		{"cbt-api", repos.CBTAPI},
		{"lab-backend", repos.LabBackend},
		{"lab", repos.Lab},
	}
	ui.Table([]string{"Repository", "Path"}, rows)

	ui.Blank()
	ui.Info(fmt.Sprintf("Lab configuration saved to: %s", configPath))

	ui.Header("Next steps:")
	fmt.Println("  1. Review and edit the 'lab:' section in .xcli.yaml if needed")
	fmt.Println("  2. Run 'xcli lab up' to start the lab stack")

	return nil
}
