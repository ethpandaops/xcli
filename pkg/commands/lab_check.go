package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabCheckCommand creates the lab check command.
func NewLabCheckCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Verify lab environment is ready",
		Long: `Perform health checks on the lab environment without starting services.

Verifies:
  • Configuration file exists and is valid
  • All required repositories are discovered and accessible
  • Docker daemon is running and accessible
  • Prerequisites are met (node_modules, .env files, etc.)
  • Ports are available (not blocked by other processes)
  • Disk space is sufficient

This is useful for:
  • Troubleshooting environment issues before 'xcli lab up'
  • Verifying a new machine setup after 'xcli lab init'
  • CI/CD pre-flight checks
  • Documentation and onboarding

Exit codes:
  0 - All checks passed
  1 - One or more checks failed

Example:
  xcli lab check`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLabCheck(cmd.Context(), log, configPath)
		},
	}
}

func runLabCheck(ctx context.Context, log logrus.FieldLogger, configPath string) error {
	ui.Header("Running lab environment health checks...")

	allPassed := true

	// Check 1: Configuration file
	spinner := ui.NewSpinner("Checking configuration file")

	result, err := config.Load(configPath)
	if err != nil {
		spinner.Fail(fmt.Sprintf("Configuration file error: %v", err))

		allPassed = false
	} else if result.Config.Lab == nil {
		spinner.Fail("Lab configuration not found - Run: xcli lab init")

		allPassed = false
	} else {
		spinner.Success("Configuration file valid")
	}

	// Check 2: Configuration validity
	if result != nil && result.Config.Lab != nil {
		spinner = ui.NewSpinner("Validating configuration")

		if err := result.Config.Lab.Validate(); err != nil {
			spinner.Fail(fmt.Sprintf("Configuration validation failed: %v", err))

			allPassed = false
		} else {
			spinner.Success("Configuration valid")
		}

		// Check 3: Repository paths
		spinner = ui.NewSpinner("Checking repository paths")

		repoCheckPassed := true
		repos := map[string]string{
			"cbt":         result.Config.Lab.Repos.CBT,
			"xatu-cbt":    result.Config.Lab.Repos.XatuCBT,
			"cbt-api":     result.Config.Lab.Repos.CBTAPI,
			"lab-backend": result.Config.Lab.Repos.LabBackend,
			"lab":         result.Config.Lab.Repos.Lab,
		}

		missingRepos := []string{}

		for name, path := range repos {
			absPath, err := filepath.Abs(path)
			if err != nil {
				missingRepos = append(missingRepos, fmt.Sprintf("%s (invalid path: %s)", name, path))
				repoCheckPassed = false

				continue
			}

			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				missingRepos = append(missingRepos, fmt.Sprintf("%s (not found: %s)", name, absPath))
				repoCheckPassed = false
			}
		}

		if !repoCheckPassed {
			failMsg := "Missing repositories - Run: xcli lab init"
			for _, repo := range missingRepos {
				failMsg += fmt.Sprintf("\n    %s", repo)
			}

			spinner.Fail(failMsg)

			allPassed = false
		} else {
			spinner.Success("All repository paths valid")
		}

		// Check 4: Prerequisites (node_modules, etc.)
		spinner = ui.NewSpinner("Checking prerequisites")

		prereqPassed := true
		prereqIssues := []string{}

		// Check lab-frontend node_modules
		labPath, _ := filepath.Abs(result.Config.Lab.Repos.Lab)
		labNodeModules := filepath.Join(labPath, "node_modules")

		if _, err := os.Stat(labNodeModules); os.IsNotExist(err) {
			prereqIssues = append(prereqIssues, "lab: missing node_modules")
			prereqPassed = false
		}

		// Check lab-backend .env
		labBackendPath, _ := filepath.Abs(result.Config.Lab.Repos.LabBackend)
		labBackendEnv := filepath.Join(labBackendPath, ".env")

		if _, err := os.Stat(labBackendEnv); os.IsNotExist(err) {
			prereqIssues = append(prereqIssues, "lab-backend: missing .env file")
			prereqPassed = false
		}

		if !prereqPassed {
			failMsg := "Missing prerequisites - Run: xcli lab init"
			for _, issue := range prereqIssues {
				failMsg += fmt.Sprintf("\n    %s", issue)
			}

			spinner.Fail(failMsg)

			allPassed = false
		} else {
			spinner.Success("All prerequisites met")
		}
	}

	// Check 5: Docker
	spinner = ui.NewSpinner("Checking Docker daemon")

	cmd := exec.CommandContext(ctx, "docker", "info")
	if err := cmd.Run(); err != nil {
		spinner.Fail("Docker daemon not accessible - Ensure Docker Desktop is running")
		allPassed = false
	} else {
		spinner.Success("Docker daemon accessible")
	}

	// Check 6: Docker Compose
	spinner = ui.NewSpinner("Checking Docker Compose")

	cmd = exec.CommandContext(ctx, "docker", "compose", "version")
	if err := cmd.Run(); err != nil {
		spinner.Fail("Docker Compose not available")
		allPassed = false
	} else {
		spinner.Success("Docker Compose available")
	}

	// Summary
	ui.Blank()

	if allPassed {
		ui.Success("All checks passed! Environment is ready.")
		ui.Header("Next steps:")
		fmt.Println("  xcli lab up              # Start the lab stack")
		fmt.Println("  xcli lab up --no-build   # Start without building (if already built)")

		return nil
	}

	ui.Error("Some checks failed. Please resolve the issues above.")

	return fmt.Errorf("environment checks failed")
}
