package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabCleanCommand creates the lab clean command.
func NewLabCleanCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove all generated artifacts and containers",
		Long: `Completely clean the lab workspace by removing all generated artifacts.

This will:
  • Stop and remove all Docker containers
  • Remove Docker volumes (data will be lost!)
  • Delete generated configuration files (.xcli/ directory)
  • Remove build artifacts (binaries in each repo)
  • Clean proto-generated files

Warning: This is a destructive operation!
  • All data in ClickHouse and Redis will be lost
  • You will need to rebuild with 'xcli lab build' or 'xcli lab up'
  • Generated configs will need to be recreated

This does NOT remove:
  • Source code or repositories
  • Your .xcli.yaml configuration file
  • node_modules or other dependencies

Use cases:
  • Starting completely fresh after config changes
  • Clearing disk space
  • Troubleshooting persistent issues
  • Switching between major configuration changes

Examples:
  xcli lab clean         # Interactive confirmation
  xcli lab clean --force # Skip confirmation prompt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLabClean(cmd.Context(), log, configPath, force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

func runLabClean(ctx context.Context, log logrus.FieldLogger, configPath string, force bool) error {
	result, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if result.Config.Lab == nil {
		return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
	}

	// Confirm unless --force
	if !force {
		ui.Warning("WARNING: This will remove all lab containers, volumes, and generated files!")
		fmt.Println("\nThis includes:")
		fmt.Println("  • All Docker containers and volumes (data will be lost)")
		fmt.Println("  • Generated configs in .xcli/ directory")
		fmt.Println("  • Build artifacts (binaries)")
		fmt.Println("  • Proto-generated files")
		fmt.Print("\nContinue? (y/N): ")

		var response string

		_, _ = fmt.Scanln(&response)

		if response != "y" && response != "Y" {
			ui.Info("Cancelled.")

			return nil
		}
	}

	ui.Header("Cleaning lab workspace...")

	// Step 1: Stop and remove containers/volumes
	ui.Header("[1/3] Stopping and removing Docker containers and volumes...")

	orch, err := orchestrator.NewOrchestrator(log, result.Config.Lab, result.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}

	if err := orch.Down(ctx); err != nil {
		ui.Warning(fmt.Sprintf("Failed to stop services: %v", err))
		ui.Info("Continuing with cleanup...")
	}

	// Step 2: Remove .xcli state directory
	ui.Header("[2/3] Removing generated configuration files...")

	configDir := filepath.Dir(result.ConfigPath)
	stateDir := filepath.Join(configDir, ".xcli")

	spinner := ui.NewSpinner("Removing generated configuration files")

	if _, err := os.Stat(stateDir); err == nil {
		if err := os.RemoveAll(stateDir); err != nil {
			spinner.Warning(fmt.Sprintf("Failed to remove %s: %v", stateDir, err))
		} else {
			spinner.Success("Generated files removed")
		}
	} else {
		spinner.Success("No generated files found")
	}

	// Step 3: Remove build artifacts
	ui.Header("[3/3] Removing build artifacts...")

	spinner = ui.NewSpinner("Removing build artifacts")

	repos := map[string]string{
		"cbt":         result.Config.Lab.Repos.CBT,
		"xatu-cbt":    result.Config.Lab.Repos.XatuCBT,
		"cbt-api":     result.Config.Lab.Repos.CBTAPI,
		"lab-backend": result.Config.Lab.Repos.LabBackend,
	}

	totalRemoved := 0

	for name, path := range repos {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}

		// Remove common build artifacts
		artifacts := []string{
			filepath.Join(absPath, name),   // Binary with repo name
			filepath.Join(absPath, "bin"),  // bin/ directory
			filepath.Join(absPath, "dist"), // dist/ directory
		}

		for _, artifact := range artifacts {
			if _, err := os.Stat(artifact); err == nil {
				if err := os.RemoveAll(artifact); err != nil {
					spinner.Warning(fmt.Sprintf("Failed to remove %s: %v", artifact, err))
				} else {
					totalRemoved++
				}
			}
		}
	}

	if totalRemoved > 0 {
		spinner.Success(fmt.Sprintf("Removed %d build artifacts", totalRemoved))
	} else {
		spinner.Success("No build artifacts found")
	}

	ui.Success("Lab workspace cleaned successfully!")
	ui.Header("Next steps:")
	fmt.Println("  xcli lab build         # Rebuild all projects")
	fmt.Println("  xcli lab up            # Build and start the stack")
	fmt.Println("  xcli lab up --rebuild  # Force rebuild and start")

	return nil
}
