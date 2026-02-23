package commands

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuCheckCommand creates the xatu check command.
func NewXatuCheckCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Verify xatu environment is ready",
		Long: `Perform health checks on the xatu environment without starting services.

Verifies:
  - Configuration file exists and is valid
  - Xatu repo exists with docker-compose.yml
  - Docker daemon is running and accessible
  - Docker Compose is available
  - Warns about potential port conflicts with lab stack

Exit codes:
  0 - All checks passed
  1 - One or more checks failed

Example:
  xcli xatu check`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runXatuCheck(cmd.Context(), log, configPath)
		},
	}
}

func runXatuCheck(ctx context.Context, log logrus.FieldLogger, configPath string) error {
	ui.Header("Running xatu environment health checks...")

	allPassed := true

	// Check 1: Configuration file
	spinner := ui.NewSpinner("Checking configuration file")

	xatuCfg, _, err := config.LoadXatuConfig(configPath)
	if err != nil {
		spinner.Fail(fmt.Sprintf("Configuration file error: %v", err))

		allPassed = false
	} else {
		spinner.Success("Configuration file valid")
	}

	// Check 2: Configuration validity
	if xatuCfg != nil {
		spinner = ui.NewSpinner("Validating configuration")

		if err := xatuCfg.Validate(); err != nil {
			spinner.Fail(fmt.Sprintf("Configuration validation failed: %v", err))

			allPassed = false
		} else {
			spinner.Success("Configuration valid")
		}

		// Check 3: docker-compose.yml
		spinner = ui.NewSpinner("Checking docker-compose.yml")

		absRepo, _ := filepath.Abs(xatuCfg.Repos.Xatu)
		composePath := filepath.Join(absRepo, "docker-compose.yml")

		if _, statErr := statFile(composePath); statErr != nil {
			spinner.Fail(fmt.Sprintf("docker-compose.yml not found: %s", composePath))

			allPassed = false
		} else {
			spinner.Success("docker-compose.yml found")
		}
	}

	// Check 4: Docker
	spinner = ui.NewSpinner("Checking Docker daemon")

	cmd := exec.CommandContext(ctx, "docker", "info")
	if err := cmd.Run(); err != nil {
		spinner.Fail("Docker daemon not accessible - Ensure Docker Desktop is running")

		allPassed = false
	} else {
		spinner.Success("Docker daemon accessible")
	}

	// Check 5: Docker Compose
	spinner = ui.NewSpinner("Checking Docker Compose")

	cmd = exec.CommandContext(ctx, "docker", "compose", "version")
	if err := cmd.Run(); err != nil {
		spinner.Fail("Docker Compose not available")

		allPassed = false
	} else {
		spinner.Success("Docker Compose available")
	}

	// Check 6: Port conflict warning with lab stack
	checkPortConflicts(log, configPath)

	// Summary
	ui.Blank()

	if allPassed {
		ui.Success("All checks passed! Environment is ready.")

		ui.Header("Next steps:")
		fmt.Println("  xcli xatu up              # Start the xatu stack")
		fmt.Println("  xcli xatu up --build      # Start with image rebuild")

		return nil
	}

	ui.Error("Some checks failed. Please resolve the issues above.")

	return fmt.Errorf("environment checks failed")
}

// checkPortConflicts warns about potential port conflicts between xatu and lab stacks.
func checkPortConflicts(log logrus.FieldLogger, configPath string) {
	result, err := config.Load(configPath)
	if err != nil || result.Config.Lab == nil || result.Config.Xatu == nil {
		return
	}

	// Both stacks are configured - warn about common port conflicts
	ui.Blank()
	ui.Warning("Both lab and xatu stacks are configured. Common port conflicts:")
	fmt.Println("  - Grafana (3000), Prometheus (9090), ClickHouse (8123)")
	ui.Info("Use xatu.envOverrides in .xcli.yaml to remap xatu ports, e.g.:")
	fmt.Println("  xatu:")
	fmt.Println("    envOverrides:")
	fmt.Println("      GRAFANA_PORT: \"3001\"")
}
