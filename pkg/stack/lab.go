package stack

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ethpandaops/xcli/pkg/builder"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/diagnostic"
	"github.com/ethpandaops/xcli/pkg/discovery"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/prerequisites"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/ethpandaops/xcli/pkg/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// labStack implements Stack for the lab development environment.
type labStack struct {
	log        logrus.FieldLogger
	configPath string

	// Flag values bound during ConfigureCommand.
	upMode         string
	upVerbose      bool
	rebuildVerbose bool
}

// NewLabStack creates a new lab stack instance.
func NewLabStack(log logrus.FieldLogger, configPath string) Stack {
	return &labStack{log: log, configPath: configPath}
}

func (s *labStack) Name() string { return "lab" }

// ConfigureCommand adds lab-specific flags and descriptions to commands.
func (s *labStack) ConfigureCommand(name string, cmd *cobra.Command) {
	switch name {
	case "init":
		cmd.Long = `Initialize the xcli lab environment by discovering repositories,
checking prerequisites, and ensuring everything is ready to start.

This command should be run once after installation, or when setting up
a new machine. It will:
  - Discover required repositories (lab, xatu-cbt, cbt-api, lab-backend, cbt)
  - Clone missing repositories
  - Check and install prerequisites (dependencies, node_modules, etc.)
  - Validate configuration

After 'xcli lab init' succeeds, you can start the stack with 'xcli lab up'.`

	case "check":
		cmd.Long = `Perform health checks on the lab environment without starting services.

Verifies:
  - Configuration file exists and is valid
  - All required repositories are discovered and accessible
  - Docker daemon is running and accessible
  - Prerequisites are met (node_modules, .env files, etc.)
  - Ports are available (not blocked by other processes)
  - Disk space is sufficient

This is useful for:
  - Troubleshooting environment issues before 'xcli lab up'
  - Verifying a new machine setup after 'xcli lab init'
  - CI/CD pre-flight checks
  - Documentation and onboarding

Exit codes:
  0 - All checks passed
  1 - One or more checks failed

Example:
  xcli lab check`

	case "up":
		cmd.Flags().StringVarP(&s.upMode, "mode", "m", "",
			"Override mode (local or hybrid)")
		cmd.Flags().BoolVarP(&s.upVerbose, "verbose", "v", false,
			"Show all build/setup command output (default: errors only)")
		cmd.Long = `Start the complete xcli lab stack including infrastructure and services.

Prerequisites must be satisfied before running this command. If you haven't
already, run 'xcli lab init' first to ensure all prerequisites are met.

The stack starts in the configured mode (local or hybrid). Use 'xcli lab mode'
to switch between modes.

This command always rebuilds all projects to ensure everything is up to date.

Flags:
  --verbose   Enable verbose output for all operations
  --mode      Override mode for this run (local or hybrid)

Examples:
  xcli lab up              # Start all services (always rebuilds)
  xcli lab up --verbose    # Startup with detailed output`

	case "down":
		cmd.Long = `Stop all running services and infrastructure in the xcli lab stack.

This will:
  - Stop all application services (CBT, cbt-api, lab-backend, frontend)
  - Stop infrastructure (ClickHouse, Redis)
  - Remove Docker containers and volumes

The stack can be restarted with 'xcli lab up'.

Example:
  xcli lab down`

	case "clean":
		cmd.Long = `Completely clean the lab workspace by removing all generated artifacts.

This will:
  - Stop and remove all Docker containers
  - Remove Docker volumes (data will be lost!)
  - Delete generated configuration files (.xcli/ directory)
  - Remove build artifacts (binaries in each repo)
  - Clean proto-generated files

Warning: This is a destructive operation!
  - All data in ClickHouse and Redis will be lost
  - You will need to rebuild with 'xcli lab build' or 'xcli lab up'
  - Generated configs will need to be recreated

This does NOT remove:
  - Source code or repositories
  - Your .xcli.yaml configuration file
  - node_modules or other dependencies

Use cases:
  - Starting completely fresh after config changes
  - Clearing disk space
  - Troubleshooting persistent issues
  - Switching between major configuration changes

Examples:
  xcli lab clean                   # Remove all containers, volumes, and build artifacts`

	case "build":
		cmd.Long = `Build all lab projects from source without starting services.

Purpose:
  This command is designed for CI/CD pipelines and pre-building scenarios.
  It compiles all binaries but does NOT start any infrastructure or services.

Use cases:
  - Pre-building before starting services
  - CI/CD build verification pipelines
  - Checking for compilation errors without running services
  - Creating clean builds from scratch

What gets built:
  Phase 1: xatu-cbt (proto definitions)
  Phase 2: CBT, lab-backend, lab-frontend (parallel)
  Phase 3: Proto generation + cbt-api (requires xatu-cbt protos)

This command always rebuilds all projects to ensure everything is up to date.

Note: This does NOT generate configs or start services. For active development
with running services, use 'xcli lab rebuild' instead.

Key difference from 'rebuild':
  - build  = Build everything, don't start services (CI/CD)
  - rebuild = Build specific component + restart its services (development)

Examples:
  xcli lab build         # Build all projects`

	case "rebuild":
		cmd.Use = "rebuild [project]"
		cmd.Flags().BoolVarP(&s.rebuildVerbose, "verbose", "v", false,
			"Enable verbose build output")
		cmd.Long = `Rebuild specific components during active development with automatic service restarts.

Purpose:
  This command is designed for rapid iteration during local development.
  It rebuilds ONLY what changed and automatically restarts affected services.

Use cases:
  - You modified code and need to test changes immediately
  - You added/changed models in xatu-cbt and need full regeneration
  - You updated API endpoints in cbt-api
  - Fast development loop without full 'down && up' cycle

Key difference from 'build':
  - build  = Build everything, don't start services (CI/CD)
  - rebuild = Build specific component + restart its services (development)

Supported projects:
  xatu-cbt     - Full rebuild and restart of ALL services
  all          - Same as 'xatu-cbt' - full rebuild and restart
                 Rebuilds: xatu-cbt, cbt, lab-backend, cbt-api
                 Restarts: cbt, cbt-api, lab-backend, lab-frontend
                 Use when: You want complete rebuild with all changes applied

  cbt          - Rebuild CBT binary + restart all CBT services
                 Use when: You modify CBT engine code

  cbt-api      - Regenerate protos + rebuild + restart all cbt-api services
                 Use when: You modify cbt-api endpoints

  lab-backend  - Rebuild + restart lab-backend service
                 Use when: You modify lab-backend code

  lab-frontend - Regenerate API types + restart lab-frontend
                 Use when: cbt-api OpenAPI spec changed

  prometheus   - Regenerate Prometheus config + restart container
                 Use when: You change scrape targets or config

  grafana      - Regenerate Grafana provisioning + restart container
                 Use when: You add/modify custom dashboards in .xcli/custom-dashboards/

Examples:
  xcli lab rebuild all               # Full rebuild and restart (alias for xatu-cbt)
  xcli lab rebuild xatu-cbt          # Full model update (same as 'all')
  xcli lab rebuild cbt               # Quick CBT engine iteration
  xcli lab rebuild lab-backend -v    # Rebuild with verbose output
  xcli lab rebuild grafana           # Reload custom dashboards

Note: All rebuild commands automatically restart their respective services if running.`

	case "status":
		cmd.Long = `Display status of all lab services and infrastructure.

Shows:
  - Running services and their states
  - Port bindings
  - Container health
  - Infrastructure status (ClickHouse, Redis)

Example:
  xcli lab status`

	case "logs":
		cmd.Long = `Show logs for all lab services or a specific service.`

	case "start":
		cmd.Long = `Start a specific lab service.

Available services:
  - lab-backend
  - lab-frontend
  - cbt-<network>        (e.g., cbt-mainnet, cbt-sepolia)
  - cbt-api-<network>    (e.g., cbt-api-mainnet, cbt-api-sepolia)

Example:
  xcli lab start lab-backend
  xcli lab start cbt-mainnet`

	case "stop":
		cmd.Long = `Stop a specific lab service.

Available services:
  - lab-backend
  - lab-frontend
  - cbt-<network>        (e.g., cbt-mainnet, cbt-sepolia)
  - cbt-api-<network>    (e.g., cbt-api-mainnet, cbt-api-sepolia)

Example:
  xcli lab stop lab-backend
  xcli lab stop cbt-mainnet`

	case "restart":
		cmd.Long = `Restart a specific lab service.`
	}
}

// CompleteServices returns a completion function for lab service names.
func (s *labStack) CompleteServices() ValidArgsFunc {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		log := logrus.New()
		log.SetOutput(io.Discard)

		labCfg, cfgPath, err := config.LoadLabConfig(s.configPath)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		orch, err := orchestrator.NewOrchestrator(log, labCfg, cfgPath)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return orch.GetValidServices(), cobra.ShellCompDirectiveNoFileComp
	}
}

// CompleteRebuildTargets returns a completion function for lab rebuild targets.
func (s *labStack) CompleteRebuildTargets() ValidArgsFunc {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return []string{
			"xatu-cbt",
			"all",
			"cbt",
			"cbt-api",
			"lab-backend",
			"lab-frontend",
			"prometheus",
			"grafana",
		}, cobra.ShellCompDirectiveNoFileComp
	}
}

// Init initializes the lab environment.
func (s *labStack) Init(ctx context.Context) error {
	ui.PrintInitBanner(version.GetVersion())

	s.log.Info("initializing lab stack")

	var (
		rootCfg            *config.Config
		resolvedConfigPath string
	)

	if _, err := os.Stat(s.configPath); err == nil {
		s.log.Info("loading existing configuration")

		result, err := config.Load(s.configPath)
		if err != nil {
			return fmt.Errorf("failed to load existing config: %w", err)
		}

		rootCfg = result.Config
		resolvedConfigPath = result.ConfigPath
	} else {
		rootCfg = &config.Config{}

		absPath, err := filepath.Abs(s.configPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute config path: %w", err)
		}

		resolvedConfigPath = absPath
	}

	if rootCfg.Lab != nil {
		s.log.Warn("lab configuration already exists")
		fmt.Print("Overwrite existing lab configuration? (y/N): ")

		var response string

		_, _ = fmt.Scanln(&response)

		if response != "y" && response != "Y" {
			s.log.Info("lab initialization cancelled")

			return nil
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	parentDir := filepath.Join(cwd, "..")
	disc := discovery.NewDiscovery(s.log, parentDir)

	ui.Header("Discovering repositories")

	spinner := ui.NewSpinner("Scanning parent directory for required repositories")

	repos, err := disc.DiscoverRepos(ctx)
	if err != nil {
		spinner.Fail("Repository discovery failed")

		return fmt.Errorf("repository discovery failed: %w", err)
	}

	spinner.Success("Found all 5 repositories")

	prereqChecker := prerequisites.NewChecker(s.log)

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

	labCfg := config.DefaultLab()
	labCfg.Repos = *repos

	rootCfg.Lab = labCfg

	if err := rootCfg.Save(resolvedConfigPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	s.log.WithField("file", resolvedConfigPath).Info("lab configuration updated")

	xcliDir := filepath.Dir(resolvedConfigPath)
	if err := config.SetXCLIPath(xcliDir); err != nil {
		s.log.WithError(err).Warn("failed to register xcli path globally (non-fatal)")
	} else {
		s.log.WithField("path", xcliDir).Info("registered xcli installation globally")
	}

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
	ui.Info(fmt.Sprintf("Lab configuration saved to: %s", s.configPath))

	ui.Header("Next steps:")
	fmt.Println("  1. Review and edit the 'lab:' section in .xcli.yaml if needed")
	fmt.Println("  2. Run 'xcli lab up' to start the lab stack")

	return nil
}

// Check verifies the lab environment is ready.
func (s *labStack) Check(ctx context.Context) error {
	ui.Header("Running lab environment health checks...")

	allPassed := true

	spinner := ui.NewSpinner("Checking configuration file")

	labCfg, _, err := config.LoadLabConfig(s.configPath)
	if err != nil {
		spinner.Fail(fmt.Sprintf("Configuration file error: %v", err))

		allPassed = false
	} else {
		spinner.Success("Configuration file valid")
	}

	if labCfg != nil {
		spinner = ui.NewSpinner("Validating configuration")

		if err := labCfg.Validate(); err != nil {
			spinner.Fail(fmt.Sprintf("Configuration validation failed: %v", err))

			allPassed = false
		} else {
			spinner.Success("Configuration valid")
		}

		spinner = ui.NewSpinner("Checking repository paths")

		repoCheckPassed := true
		repos := map[string]string{
			"cbt":         labCfg.Repos.CBT,
			"xatu-cbt":    labCfg.Repos.XatuCBT,
			"cbt-api":     labCfg.Repos.CBTAPI,
			"lab-backend": labCfg.Repos.LabBackend,
			"lab":         labCfg.Repos.Lab,
		}

		missingRepos := []string{}

		for name, path := range repos {
			absPath, err := filepath.Abs(path)
			if err != nil {
				missingRepos = append(missingRepos,
					fmt.Sprintf("%s (invalid path: %s)", name, path))
				repoCheckPassed = false

				continue
			}

			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				missingRepos = append(missingRepos,
					fmt.Sprintf("%s (not found: %s)", name, absPath))
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

		spinner = ui.NewSpinner("Checking prerequisites")

		prereqPassed := true
		prereqIssues := []string{}

		labPath, _ := filepath.Abs(labCfg.Repos.Lab)
		labNodeModules := filepath.Join(labPath, "node_modules")

		if _, err := os.Stat(labNodeModules); os.IsNotExist(err) {
			prereqIssues = append(prereqIssues, "lab: missing node_modules")
			prereqPassed = false
		}

		labBackendPath, _ := filepath.Abs(labCfg.Repos.LabBackend)
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

	spinner = ui.NewSpinner("Checking Docker daemon")

	cmd := exec.CommandContext(ctx, "docker", "info")
	if err := cmd.Run(); err != nil {
		spinner.Fail("Docker daemon not accessible - Ensure Docker Desktop is running")

		allPassed = false
	} else {
		spinner.Success("Docker daemon accessible")
	}

	spinner = ui.NewSpinner("Checking Docker Compose")

	cmd = exec.CommandContext(ctx, "docker", "compose", "version")
	if err := cmd.Run(); err != nil {
		spinner.Fail("Docker Compose not available")

		allPassed = false
	} else {
		spinner.Success("Docker compose available")
	}

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

// Up starts the lab stack.
func (s *labStack) Up(ctx context.Context) error {
	labCfg, cfgPath, err := config.LoadLabConfig(s.configPath)
	if err != nil {
		return err
	}

	if s.upMode != "" {
		labCfg.Mode = s.upMode
	}

	if validationErr := labCfg.Validate(); validationErr != nil {
		return fmt.Errorf("invalid lab configuration: %w", validationErr)
	}

	orch, err := orchestrator.NewOrchestrator(s.log, labCfg, cfgPath)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}

	orch.SetVerbose(s.upVerbose)

	err = orch.Up(ctx, false, true, nil)

	if err != nil && errors.Is(err, context.Canceled) {
		ui.Warning("Interrupt received, shutting down gracefully...")

		cleanupCtx := context.Background()

		if stopErr := orch.StopServices(cleanupCtx, nil); stopErr != nil {
			s.log.WithError(stopErr).Error("failed to stop services gracefully")
		} else {
			ui.Success("All services stopped")
		}

		return nil
	}

	return err
}

// Down stops the lab stack.
func (s *labStack) Down(ctx context.Context) error {
	labCfg, cfgPath, err := config.LoadLabConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	orch, err := orchestrator.NewOrchestrator(s.log, labCfg, cfgPath)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}

	return orch.Down(ctx, nil)
}

// Clean removes all lab containers, volumes, and build artifacts.
func (s *labStack) Clean(ctx context.Context) error {
	labCfg, cfgPath, err := config.LoadLabConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	ui.Warning("WARNING: This will remove all lab containers, volumes, and generated files!")
	fmt.Println("\nThis includes:")
	fmt.Println("  - All Docker containers and volumes (data will be lost)")
	fmt.Println("  - Generated configs in .xcli/ directory")
	fmt.Println("  - Build artifacts (binaries)")
	fmt.Println("  - Proto-generated files")
	fmt.Print("\nContinue? (y/N): ")

	var response string

	_, _ = fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		ui.Info("Cancelled.")

		return nil
	}

	ui.Header("Cleaning lab workspace...")

	ui.Header("[1/3] Stopping and removing Docker containers and volumes...")

	orch, err := orchestrator.NewOrchestrator(s.log, labCfg, cfgPath)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}

	if err := orch.Down(ctx, nil); err != nil {
		ui.Warning(fmt.Sprintf("Failed to stop services: %v", err))
		ui.Info("Continuing with cleanup...")
	}

	ui.Header("[2/3] Removing generated configuration files...")

	configDir := filepath.Dir(cfgPath)
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

	ui.Header("[3/3] Removing build artifacts...")

	spinner = ui.NewSpinner("Removing build artifacts")

	repos := map[string]string{
		"cbt":         labCfg.Repos.CBT,
		"xatu-cbt":    labCfg.Repos.XatuCBT,
		"cbt-api":     labCfg.Repos.CBTAPI,
		"lab-backend": labCfg.Repos.LabBackend,
	}

	totalRemoved := 0

	for name, path := range repos {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}

		artifacts := []string{
			filepath.Join(absPath, name),
			filepath.Join(absPath, "bin"),
			filepath.Join(absPath, "dist"),
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
	ui.Header("Next step:")
	fmt.Println("  xcli lab up            # Build and start the stack")

	return nil
}

// Build builds all lab projects from source without starting services.
func (s *labStack) Build(ctx context.Context, _ []string) error {
	labCfg, cfgPath, err := config.LoadLabConfig(s.configPath)
	if err != nil {
		return err
	}

	if validateErr := labCfg.ValidateRepos(); validateErr != nil {
		return fmt.Errorf("invalid lab configuration: %w", validateErr)
	}

	absConfigPath, err := filepath.Abs(cfgPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute config path: %w", err)
	}

	configDir := filepath.Dir(absConfigPath)
	stateDir := filepath.Join(configDir, ".xcli")

	buildMgr := builder.NewManager(s.log, labCfg, stateDir)

	ui.Header("Building all lab repositories")
	ui.Blank()

	if err := buildMgr.BuildAll(ctx, true); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	ui.Success("Build complete!")
	ui.Info("Note: cbt-api protos not generated (requires infrastructure).")
	ui.Info("Run 'xcli lab up' to start infrastructure and complete the build.")

	return nil
}

// Rebuild rebuilds a specific lab component and restarts affected services.
func (s *labStack) Rebuild(ctx context.Context, project string) error {
	labCfg, cfgPath, err := config.LoadLabConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	absConfigPath, err := filepath.Abs(cfgPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute config path: %w", err)
	}

	configDir := filepath.Dir(absConfigPath)
	stateDir := filepath.Join(configDir, ".xcli")

	orch, err := orchestrator.NewOrchestrator(s.log, labCfg, cfgPath)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}

	orch.SetVerbose(s.rebuildVerbose)

	switch project {
	case "xatu-cbt", "all":
		return s.runFullRebuild(ctx, orch, stateDir)

	case "cbt":
		spinner := ui.NewSpinner("Rebuilding CBT")

		if err := orch.Builder().BuildCBT(ctx, true); err != nil {
			spinner.Fail("Failed to rebuild CBT")

			return fmt.Errorf("failed to rebuild CBT: %w", err)
		}

		spinner.Success("CBT rebuilt successfully")

		enabledNetworks := orch.Config().EnabledNetworks()
		for _, network := range enabledNetworks {
			serviceName := fmt.Sprintf("cbt-%s", network.Name)
			spinner = ui.NewSpinner(fmt.Sprintf("Restarting %s", serviceName))

			if err := orch.Restart(ctx, serviceName); err != nil {
				spinner.Warning(fmt.Sprintf("Could not restart %s", serviceName))
			} else {
				spinner.Success(fmt.Sprintf("%s restarted", serviceName))
			}
		}

	case "cbt-api":
		spinner := ui.NewSpinner("Regenerating protos and rebuilding cbt-api")

		if err := orch.Builder().BuildCBTAPI(ctx, true); err != nil {
			spinner.Fail("Failed to rebuild cbt-api")

			return fmt.Errorf("failed to rebuild cbt-api: %w", err)
		}

		spinner.Success("cbt-api rebuilt successfully")

		enabledNetworks := orch.Config().EnabledNetworks()
		for _, network := range enabledNetworks {
			serviceName := fmt.Sprintf("cbt-api-%s", network.Name)
			spinner = ui.NewSpinner(fmt.Sprintf("Restarting %s", serviceName))

			if err := orch.Restart(ctx, serviceName); err != nil {
				spinner.Warning(fmt.Sprintf("Could not restart %s", serviceName))
			} else {
				spinner.Success(fmt.Sprintf("%s restarted", serviceName))
			}
		}

		ui.Blank()
		ui.Info("Note: If you added models in xatu-cbt, use " +
			"'xcli lab rebuild xatu-cbt' for full workflow")

	case "lab-backend":
		spinner := ui.NewSpinner("Rebuilding lab-backend")

		if err := orch.Builder().BuildLabBackend(ctx, true); err != nil {
			spinner.Fail("Failed to rebuild lab-backend")

			return fmt.Errorf("failed to rebuild lab-backend: %w", err)
		}

		spinner.Success("lab-backend rebuilt successfully")

		spinner = ui.NewSpinner("Restarting lab-backend")

		if err := orch.Restart(ctx, "lab-backend"); err != nil {
			spinner.Fail("Could not restart lab-backend")
			ui.Info("If lab-backend is running, restart it manually:")
			ui.Info("  xcli lab restart lab-backend")
		} else {
			spinner.Success("lab-backend restarted successfully")
		}

	case "lab-frontend":
		spinner := ui.NewSpinner("Regenerating lab-frontend API types from cbt-api")

		if err := orch.Builder().BuildLabFrontend(ctx); err != nil {
			spinner.Fail("Failed to regenerate lab-frontend types")

			return fmt.Errorf("failed to regenerate lab-frontend types: %w", err)
		}

		spinner.Success("lab-frontend API types regenerated successfully")

		spinner = ui.NewSpinner("Restarting lab-frontend")

		if err := orch.Restart(ctx, "lab-frontend"); err != nil {
			spinner.Fail("Could not restart lab-frontend")
			ui.Info("If lab-frontend is running, restart it manually:")
			ui.Info("  xcli lab restart lab-frontend")
		} else {
			spinner.Success("lab-frontend restarted successfully")
		}

	case "prometheus":
		if !labCfg.Infrastructure.Observability.Enabled {
			return fmt.Errorf("observability is not enabled in config")
		}

		spinner := ui.NewSpinner("Regenerating Prometheus config")

		if err := orch.RebuildObservability(ctx, "prometheus"); err != nil {
			spinner.Fail("Failed to rebuild Prometheus")

			return fmt.Errorf("failed to rebuild prometheus: %w", err)
		}

		spinner.Success("Prometheus config regenerated and container restarted")

	case "grafana":
		if !labCfg.Infrastructure.Observability.Enabled {
			return fmt.Errorf("observability is not enabled in config")
		}

		spinner := ui.NewSpinner("Regenerating Grafana provisioning and dashboards")

		if err := orch.RebuildObservability(ctx, "grafana"); err != nil {
			spinner.Fail("Failed to rebuild Grafana")

			return fmt.Errorf("failed to rebuild grafana: %w", err)
		}

		spinner.Success("Grafana provisioning regenerated and container restarted")
		ui.Info("Custom dashboards loaded from .xcli/custom-dashboards/")

	default:
		return fmt.Errorf(
			"unknown project: %s\n\nSupported projects: "+
				"xatu-cbt, cbt, cbt-api, lab-backend, lab-frontend, "+
				"prometheus, grafana, all",
			project,
		)
	}

	return nil
}

// PrintStatus displays lab stack status.
func (s *labStack) PrintStatus(ctx context.Context) error {
	labCfg, cfgPath, err := config.LoadLabConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	orch, err := orchestrator.NewOrchestrator(s.log, labCfg, cfgPath)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}

	return orch.Status(ctx)
}

// Logs shows logs for lab services.
func (s *labStack) Logs(ctx context.Context, service string, follow bool) error {
	labCfg, cfgPath, err := config.LoadLabConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	orch, err := orchestrator.NewOrchestrator(s.log, labCfg, cfgPath)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}

	return orch.Logs(ctx, service, follow)
}

// Start starts a specific lab service.
func (s *labStack) Start(ctx context.Context, service string) error {
	labCfg, cfgPath, err := config.LoadLabConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	orch, err := orchestrator.NewOrchestrator(s.log, labCfg, cfgPath)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}

	if !orch.IsValidService(service) {
		ui.Error(fmt.Sprintf("Unknown service: %s", service))
		fmt.Println("\nAvailable services:")

		for _, svc := range orch.GetValidServices() {
			fmt.Printf("  - %s\n", svc)
		}

		return fmt.Errorf("unknown service: %s", service)
	}

	spinner := ui.NewSpinner(fmt.Sprintf("Starting %s", service))

	if err := orch.StartService(ctx, service); err != nil {
		spinner.Fail(fmt.Sprintf("Failed to start %s", service))

		return fmt.Errorf("failed to start service: %w", err)
	}

	spinner.Success(fmt.Sprintf("%s started successfully", service))

	return nil
}

// Stop stops a specific lab service.
func (s *labStack) Stop(ctx context.Context, service string) error {
	labCfg, cfgPath, err := config.LoadLabConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	orch, err := orchestrator.NewOrchestrator(s.log, labCfg, cfgPath)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}

	if !orch.IsValidService(service) {
		ui.Error(fmt.Sprintf("Unknown service: %s", service))
		fmt.Println("\nAvailable services:")

		for _, svc := range orch.GetValidServices() {
			fmt.Printf("  - %s\n", svc)
		}

		return fmt.Errorf("unknown service: %s", service)
	}

	spinner := ui.NewSpinner(fmt.Sprintf("Stopping %s", service))

	if err := orch.StopService(ctx, service); err != nil {
		spinner.Fail(fmt.Sprintf("Failed to stop %s", service))

		return fmt.Errorf("failed to stop service: %w", err)
	}

	spinner.Success(fmt.Sprintf("%s stopped successfully", service))

	return nil
}

// Restart restarts a specific lab service.
func (s *labStack) Restart(ctx context.Context, service string) error {
	labCfg, cfgPath, err := config.LoadLabConfig(s.configPath)
	if err != nil {
		return err
	}

	orch, err := orchestrator.NewOrchestrator(s.log, labCfg, cfgPath)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}

	if !orch.IsValidService(service) {
		ui.Error(fmt.Sprintf("Unknown service: %s", service))
		fmt.Println("\nAvailable services:")

		for _, svc := range orch.GetValidServices() {
			fmt.Printf("  - %s\n", svc)
		}

		return fmt.Errorf("unknown service: %s", service)
	}

	spinner := ui.NewSpinner(fmt.Sprintf("Restarting %s", service))

	if err := orch.Restart(ctx, service); err != nil {
		spinner.Fail(fmt.Sprintf("Failed to restart %s", service))

		return fmt.Errorf("failed to restart service: %w", err)
	}

	spinner.Success(fmt.Sprintf("%s restarted successfully", service))

	return nil
}

// runFullRebuild executes the full rebuild and restart workflow.
func (s *labStack) runFullRebuild(
	ctx context.Context,
	orch *orchestrator.Orchestrator,
	stateDir string,
) error {
	report := diagnostic.NewRebuildReport()
	store := diagnostic.NewStore(s.log, filepath.Join(stateDir, "errors"))

	ui.Header("Starting full rebuild and restart workflow")
	fmt.Println("This will:")
	fmt.Println("  - Regenerate all protos (xatu-cbt, cbt-api)")
	fmt.Println("  - Rebuild all binaries (xatu-cbt, cbt, cbt-api, lab-backend)")
	fmt.Println("  - Regenerate configs")
	fmt.Println("  - Restart all services")
	fmt.Println("  - Regenerate lab-frontend types")
	ui.Blank()

	protoGenFailed := false
	xatuCBTBuildFailed := false
	cbtAPIFailed := false

	// Step 1: Regenerate xatu-cbt protos
	spinner := ui.NewSpinner("[1/7] Regenerating xatu-cbt protos")

	result := orch.Builder().GenerateXatuCBTProtosWithResult(ctx)
	report.AddResult(*result)

	if !result.Success {
		spinner.Fail("Failed to regenerate xatu-cbt protos")

		protoGenFailed = true
	} else {
		spinner.Success("xatu-cbt protos regenerated")
	}

	// Step 2: Rebuild xatu-cbt binary
	if protoGenFailed {
		now := time.Now()
		report.AddResult(diagnostic.BuildResult{
			Phase:     diagnostic.PhaseBuild,
			Service:   "xatu-cbt",
			Success:   false,
			ErrorMsg:  "skipped due to proto generation failure",
			StartTime: now,
			EndTime:   now,
		})

		xatuCBTBuildFailed = true
	} else {
		spinner = ui.NewSpinner("[2/7] Rebuilding xatu-cbt")

		result = orch.Builder().BuildXatuCBTWithResult(ctx, true)
		report.AddResult(*result)

		if !result.Success {
			spinner.Fail("Failed to rebuild xatu-cbt")

			xatuCBTBuildFailed = true
		} else {
			spinner.Success("xatu-cbt rebuilt")
		}
	}

	// Step 3: Regenerate cbt-api protos + rebuild cbt-api
	if xatuCBTBuildFailed {
		now := time.Now()
		report.AddResult(diagnostic.BuildResult{
			Phase:     diagnostic.PhaseBuild,
			Service:   "cbt-api",
			Success:   false,
			ErrorMsg:  "skipped due to xatu-cbt build failure",
			StartTime: now,
			EndTime:   now,
		})

		cbtAPIFailed = true
	} else {
		spinner = ui.NewSpinner(
			"[3/7] Regenerating cbt-api protos and rebuilding cbt-api")

		result = orch.Builder().BuildCBTAPIWithResult(ctx, true)
		report.AddResult(*result)

		if !result.Success {
			spinner.Fail("Failed to rebuild cbt-api")

			cbtAPIFailed = true
		} else {
			spinner.Success("cbt-api protos regenerated and cbt-api rebuilt")
		}
	}

	// Step 4: Rebuild remaining binaries (cbt, lab-backend)
	spinner = ui.NewSpinner(
		"[4/7] Rebuilding remaining binaries (cbt, lab-backend)")

	result = orch.Builder().BuildCBTWithResult(ctx, true)
	report.AddResult(*result)

	cbtFailed := !result.Success

	result = orch.Builder().BuildLabBackendWithResult(ctx, true)
	report.AddResult(*result)

	labBackendFailed := !result.Success

	if cbtFailed && labBackendFailed {
		spinner.Fail("Failed to rebuild cbt and lab-backend")
	} else if cbtFailed {
		spinner.Fail("Failed to rebuild cbt")
	} else if labBackendFailed {
		spinner.Fail("Failed to rebuild lab-backend")
	} else {
		spinner.Success("Remaining binaries rebuilt (cbt, lab-backend)")
	}

	// Step 5: Regenerate configs
	spinner = ui.NewSpinner("[5/7] Regenerating configs")

	configStart := time.Now()
	configErr := orch.GenerateConfigs(ctx)
	configEnd := time.Now()

	configResult := diagnostic.BuildResult{
		Phase:     diagnostic.PhaseConfigGen,
		Service:   "configs",
		Success:   configErr == nil,
		StartTime: configStart,
		EndTime:   configEnd,
		Duration:  configEnd.Sub(configStart),
	}
	if configErr != nil {
		configResult.Error = configErr
		configResult.ErrorMsg = configErr.Error()
	}

	report.AddResult(configResult)

	if configErr != nil {
		spinner.Fail("Failed to regenerate configs")
	} else {
		spinner.Success("Configs regenerated")
	}

	// Step 6: Restart ALL services
	spinner = ui.NewSpinner(
		"[6/7] Restarting all services (cbt-api + CBT engines + lab-backend)")

	if !orch.AreServicesRunning() {
		_ = spinner.Stop()

		now := time.Now()
		report.AddResult(diagnostic.BuildResult{
			Phase:     diagnostic.PhaseRestart,
			Service:   "all-services",
			Success:   true,
			ErrorMsg:  "services not running - skipped",
			StartTime: now,
			EndTime:   now,
		})

		report.Finalize()
		ui.Blank()
		ui.DisplayBuildSummary(report)

		if err := store.Save(report); err != nil {
			s.log.WithError(err).Warn("Failed to save diagnostic report")
		}

		if report.HasFailures() {
			ui.Blank()
			ui.Info("Run 'xcli lab diagnose' for error analysis")
			ui.Info("Run 'xcli lab diagnose --ai' for AI-powered diagnosis")

			return fmt.Errorf("rebuild failed: %d of %d steps failed",
				report.FailedCount, report.TotalCount)
		}

		ui.Blank()
		ui.Warning("Services not currently running - " +
			"skipping restart and lab-frontend regeneration")
		ui.Info("All binaries, protos and configs have been regenerated.")
		ui.Info("Start services with: xcli lab up")

		return nil
	}

	restartStart := time.Now()
	restartErr := orch.RestartAllServices(ctx, s.rebuildVerbose)
	restartEnd := time.Now()

	restartResult := diagnostic.BuildResult{
		Phase:     diagnostic.PhaseRestart,
		Service:   "all-services",
		Success:   restartErr == nil,
		StartTime: restartStart,
		EndTime:   restartEnd,
		Duration:  restartEnd.Sub(restartStart),
	}
	if restartErr != nil {
		restartResult.Error = restartErr
		restartResult.ErrorMsg = restartErr.Error()
	}

	report.AddResult(restartResult)

	if restartErr != nil {
		spinner.Fail("Failed to restart services")
	} else {
		spinner.Success("All services restarted")
	}

	// Step 7: Regenerate lab-frontend types
	if restartErr != nil || cbtAPIFailed {
		now := time.Now()
		skipReason := "skipped due to restart failure"

		if cbtAPIFailed {
			skipReason = "skipped due to cbt-api build failure"
		}

		report.AddResult(diagnostic.BuildResult{
			Phase:     diagnostic.PhaseFrontendGen,
			Service:   "lab-frontend",
			Success:   false,
			ErrorMsg:  skipReason,
			StartTime: now,
			EndTime:   now,
		})
	} else {
		spinner = ui.NewSpinner("[7/7] Regenerating lab-frontend API types")

		spinner.UpdateText("[7/7] Waiting for cbt-api to be ready")

		waitStart := time.Now()

		waitErr := orch.WaitForCBTAPIReady(ctx)
		if waitErr != nil {
			spinner.Fail("cbt-api did not become ready")

			waitEnd := time.Now()
			report.AddResult(diagnostic.BuildResult{
				Phase:   diagnostic.PhaseFrontendGen,
				Service: "lab-frontend",
				Success: false,
				Error:   waitErr,
				ErrorMsg: fmt.Sprintf(
					"cbt-api did not become ready: %v", waitErr),
				StartTime: waitStart,
				EndTime:   waitEnd,
				Duration:  waitEnd.Sub(waitStart),
			})
		} else {
			spinner.UpdateText("[7/7] Regenerating lab-frontend API types")

			result = orch.Builder().BuildLabFrontendWithResult(ctx)
			report.AddResult(*result)

			if !result.Success {
				spinner.Fail("Failed to regenerate lab-frontend types")
			} else {
				spinner.UpdateText("[7/7] Restarting lab-frontend")

				if err := orch.Restart(ctx, "lab-frontend"); err != nil {
					spinner.Fail("Could not restart lab-frontend")
					ui.Warning(fmt.Sprintf(
						"Could not restart lab-frontend: %v", err))
					ui.Info("If lab-frontend is running, restart it manually:")
					ui.Info("  xcli lab restart lab-frontend")
				} else {
					spinner.Success(
						"Lab-frontend API types regenerated and service restarted")
				}
			}
		}
	}

	report.Finalize()
	ui.Blank()
	ui.DisplayBuildSummary(report)

	if err := store.Save(report); err != nil {
		s.log.WithError(err).Warn("Failed to save diagnostic report")
	}

	if report.HasFailures() {
		ui.Blank()
		ui.Info("Run 'xcli lab diagnose' for error analysis")
		ui.Info("Run 'xcli lab diagnose --ai' for AI-powered diagnosis")

		return fmt.Errorf("rebuild failed: %d of %d steps failed",
			report.FailedCount, report.TotalCount)
	}

	ui.Blank()
	ui.Success("Full rebuild and restart complete")

	return nil
}
