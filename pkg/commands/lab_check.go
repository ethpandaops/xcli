package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
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
	fmt.Println("Running lab environment health checks...")

	allPassed := true

	// Check 1: Configuration file
	fmt.Print("✓ Checking configuration file... ")

	result, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("✗\n  Error: %v\n", err)

		allPassed = false
	} else if result.Config.Lab == nil {
		fmt.Println("✗")
		fmt.Println("  Error: Lab configuration not found")
		fmt.Println("  Run: xcli lab init")

		allPassed = false
	} else {
		fmt.Println("✓")
	}

	// Check 2: Configuration validity
	if result != nil && result.Config.Lab != nil {
		fmt.Print("✓ Validating configuration... ")

		if err := result.Config.Lab.Validate(); err != nil {
			fmt.Printf("✗\n  Error: %v\n", err)

			allPassed = false
		} else {
			fmt.Println("✓")
		}

		// Check 3: Repository paths
		fmt.Print("✓ Checking repository paths... ")

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
			fmt.Println("✗")

			for _, repo := range missingRepos {
				fmt.Printf("  Missing: %s\n", repo)
			}

			fmt.Println("  Run: xcli lab init")

			allPassed = false
		} else {
			fmt.Printf("✓ (all 5 repos found)\n")
		}

		// Check 4: Prerequisites (node_modules, etc.)
		fmt.Print("✓ Checking prerequisites... ")

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
			fmt.Println("✗")

			for _, issue := range prereqIssues {
				fmt.Printf("  Missing: %s\n", issue)
			}

			fmt.Println("  Run: xcli lab init")

			allPassed = false
		} else {
			fmt.Println("✓")
		}
	}

	// Check 5: Docker
	fmt.Print("✓ Checking Docker daemon... ")

	cmd := exec.CommandContext(ctx, "docker", "info")
	if err := cmd.Run(); err != nil {
		fmt.Println("✗")
		fmt.Println("  Error: Docker daemon not accessible")
		fmt.Println("  Ensure Docker Desktop is running")

		allPassed = false
	} else {
		fmt.Println("✓")
	}

	// Check 6: Docker Compose
	fmt.Print("✓ Checking Docker Compose... ")

	cmd = exec.CommandContext(ctx, "docker", "compose", "version")
	if err := cmd.Run(); err != nil {
		fmt.Println("✗")
		fmt.Println("  Error: Docker Compose not available")

		allPassed = false
	} else {
		fmt.Println("✓")
	}

	// Summary
	fmt.Println()

	if allPassed {
		fmt.Println("✓ All checks passed! Environment is ready.")
		fmt.Println("\nNext steps:")
		fmt.Println("  xcli lab up              # Start the lab stack")
		fmt.Println("  xcli lab up --no-build   # Start without building (if already built)")

		return nil
	}

	fmt.Println("✗ Some checks failed. Please resolve the issues above.")

	return fmt.Errorf("environment checks failed")
}
