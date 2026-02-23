package stack

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/compose"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/ethpandaops/xcli/pkg/version"
	"github.com/pterm/pterm"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// xatuStack implements Stack for the xatu docker-compose stack.
type xatuStack struct {
	log        logrus.FieldLogger
	configPath string

	// Flag values bound during ConfigureCommand.
	upBuild     bool
	downVolumes bool
}

// NewXatuStack creates a new xatu stack instance.
func NewXatuStack(log logrus.FieldLogger, configPath string) Stack {
	return &xatuStack{log: log, configPath: configPath}
}

func (s *xatuStack) Name() string { return "xatu" }

// ConfigureCommand adds xatu-specific flags and descriptions to commands.
func (s *xatuStack) ConfigureCommand(name string, cmd *cobra.Command) {
	switch name {
	case "init":
		cmd.Long = `Initialize the xatu stack environment by discovering the xatu repository,
verifying Docker and Docker Compose are available, and saving configuration.

This command will:
  - Search for the xatu repo at ../xatu (relative to config file)
  - Verify docker-compose.yml exists in the repo
  - Check Docker and Docker Compose are available
  - Save xatu configuration to .xcli.yaml

After 'xcli xatu init' succeeds, you can start the stack with 'xcli xatu up'.`

	case "check":
		cmd.Long = `Perform health checks on the xatu environment without starting services.

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
  xcli xatu check`

	case "up":
		cmd.Flags().BoolVar(&s.upBuild, "build", false,
			"Build images before starting containers")
		cmd.Long = `Start the xatu docker-compose stack.

This runs 'docker compose up -d' in the xatu repository directory,
using any configured profiles and environment overrides.

Flags:
  --build   Build images before starting containers

Examples:
  xcli xatu up              # Start all services
  xcli xatu up --build      # Build images and start`

	case "down":
		// No shorthand -v: conflicts with root persistent --verbose flag.
		cmd.Flags().BoolVar(&s.downVolumes, "volumes", false,
			"Remove named volumes")
		cmd.Long = `Stop all running services in the xatu docker-compose stack.

This runs 'docker compose down' in the xatu repository directory.

Flags:
  --volumes   Remove named volumes declared in the volumes section

Examples:
  xcli xatu down              # Stop all services
  xcli xatu down --volumes    # Stop and remove volumes`

	case "clean":
		cmd.Long = `Completely clean the xatu docker-compose stack.

This will:
  - Stop and remove all containers
  - Remove all named volumes (data will be lost!)
  - Remove all images built by the stack

Warning: This is a destructive operation!
  - All data in ClickHouse, Kafka, etc. will be lost
  - All locally built images will be removed
  - You will need to rebuild with 'xcli xatu up --build'

This does NOT remove:
  - Source code or the xatu repository
  - Your .xcli.yaml configuration file

Examples:
  xcli xatu clean`

	case "build":
		cmd.Use = "build [service...]"
		cmd.ValidArgsFunction = s.CompleteServices()
		cmd.Long = `Build docker images for xatu services without starting them.

If no service is specified, all services with build configurations are built.

Examples:
  xcli xatu build                   # Build all images
  xcli xatu build xatu-server       # Build just xatu-server image`

	case "rebuild":
		cmd.Use = "rebuild <service>"
		cmd.Long = `Rebuild a service's docker image and restart it.

This builds the image with 'docker compose build <service>' (showing full
build output) then starts it with 'docker compose up -d <service>'.

For source-built services (xatu-server, xatu-cannon, xatu-sentry-logs, etc.)
this rebuilds from the Dockerfile. For image-based services (clickhouse, kafka, etc.)
this recreates the container with the latest image.

Examples:
  xcli xatu rebuild xatu-server         # Rebuild and restart xatu-server
  xcli xatu rebuild xatu-cannon         # Rebuild and restart xatu-cannon`

	case "status":
		cmd.Long = `Display status of all xatu services.

Shows:
  - Running services and their states
  - Port bindings
  - Container health

Example:
  xcli xatu status`

	case "logs":
		cmd.Long = `Show logs for all xatu services or a specific service.

Examples:
  xcli xatu logs                    # Show logs for all services
  xcli xatu logs xatu-server        # Show logs for xatu-server
  xcli xatu logs xatu-server -f     # Follow xatu-server logs`

	case "start":
		cmd.Long = `Start a specific xatu service that was previously stopped.

Example:
  xcli xatu start xatu-server
  xcli xatu start clickhouse`

	case "stop":
		cmd.Long = `Stop a specific xatu service.

Example:
  xcli xatu stop xatu-server
  xcli xatu stop clickhouse`

	case "restart":
		cmd.Long = `Restart a specific xatu service.

Example:
  xcli xatu restart xatu-server
  xcli xatu restart clickhouse`
	}
}

// CompleteServices returns a completion function for xatu service names.
func (s *xatuStack) CompleteServices() ValidArgsFunc {
	return func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		log := logrus.New()
		log.SetOutput(io.Discard)

		xatuCfg, _, err := config.LoadXatuConfig(s.configPath)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		runner, err := compose.NewRunner(
			log, xatuCfg.Repos.Xatu, xatuCfg.Profiles, xatuCfg.EnvOverrides)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		services, err := runner.ListServices(cmd.Context())
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return services, cobra.ShellCompDirectiveNoFileComp
	}
}

// CompleteRebuildTargets returns the same completion as CompleteServices
// since xatu rebuild targets are docker-compose service names.
func (s *xatuStack) CompleteRebuildTargets() ValidArgsFunc {
	return s.CompleteServices()
}

// Init initializes the xatu stack environment.
func (s *xatuStack) Init(ctx context.Context) error {
	ui.PrintInitBanner(version.GetVersion())

	s.log.Info("initializing xatu stack")

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

	if rootCfg.Xatu != nil {
		s.log.Warn("xatu configuration already exists")
		fmt.Print("Overwrite existing xatu configuration? (y/N): ")

		var response string

		_, _ = fmt.Scanln(&response)

		if response != "y" && response != "Y" {
			s.log.Info("xatu initialization cancelled")

			return nil
		}
	}

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
			return fmt.Errorf(
				"xatu repository is required - clone it manually or run init again")
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

	spinner = ui.NewSpinner("Checking docker-compose.yml")

	composePath := filepath.Join(absXatuPath, "docker-compose.yml")
	if _, statErr := os.Stat(composePath); os.IsNotExist(statErr) {
		spinner.Fail("docker-compose.yml not found in xatu repo")

		return fmt.Errorf("docker-compose.yml not found at: %s", composePath)
	}

	spinner.Success("docker-compose.yml found")

	ui.Header("Checking prerequisites")

	spinner = ui.NewSpinner("Checking Docker daemon")

	if dockerErr := exec.CommandContext(ctx, "docker", "info").Run(); dockerErr != nil {
		spinner.Fail("Docker daemon not accessible - Ensure Docker Desktop is running")

		return fmt.Errorf("docker is required but not available: %w", dockerErr)
	}

	spinner.Success("Docker daemon accessible")

	spinner = ui.NewSpinner("Checking Docker Compose")

	if composeErr := exec.CommandContext(ctx, "docker", "compose", "version").Run(); composeErr != nil {
		spinner.Fail("Docker Compose not available")

		return fmt.Errorf("docker compose is required but not available: %w", composeErr)
	}

	spinner.Success("Docker Compose available")

	xatuCfg := config.DefaultXatu()
	xatuCfg.Repos.Xatu = absXatuPath

	rootCfg.Xatu = xatuCfg

	if err := rootCfg.Save(resolvedConfigPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	s.log.WithField("file", resolvedConfigPath).Info("xatu configuration updated")

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

// Check verifies the xatu environment is ready.
func (s *xatuStack) Check(ctx context.Context) error {
	ui.Header("Running xatu environment health checks...")

	allPassed := true

	spinner := ui.NewSpinner("Checking configuration file")

	xatuCfg, _, err := config.LoadXatuConfig(s.configPath)
	if err != nil {
		spinner.Fail(fmt.Sprintf("Configuration file error: %v", err))

		allPassed = false
	} else {
		spinner.Success("Configuration file valid")
	}

	if xatuCfg != nil {
		spinner = ui.NewSpinner("Validating configuration")

		if err := xatuCfg.Validate(); err != nil {
			spinner.Fail(fmt.Sprintf("Configuration validation failed: %v", err))

			allPassed = false
		} else {
			spinner.Success("Configuration valid")
		}

		spinner = ui.NewSpinner("Checking docker-compose.yml")

		absRepo, _ := filepath.Abs(xatuCfg.Repos.Xatu)
		composePath := filepath.Join(absRepo, "docker-compose.yml")

		if _, statErr := os.Stat(composePath); os.IsNotExist(statErr) {
			spinner.Fail(fmt.Sprintf(
				"docker-compose.yml not found: %s", composePath))

			allPassed = false
		} else {
			spinner.Success("docker-compose.yml found")
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
		spinner.Success("Docker Compose available")
	}

	s.checkPortConflicts()

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

// Up starts the xatu docker-compose stack.
func (s *xatuStack) Up(ctx context.Context) error {
	xatuCfg, _, err := config.LoadXatuConfig(s.configPath)
	if err != nil {
		return err
	}

	if validationErr := xatuCfg.Validate(); validationErr != nil {
		return fmt.Errorf("invalid xatu configuration: %w", validationErr)
	}

	runner, err := newXatuRunner(s.log, xatuCfg)
	if err != nil {
		return fmt.Errorf("failed to create compose runner: %w", err)
	}

	ui.Header("Starting xatu stack...")
	ui.Blank()

	if err := runner.Up(ctx, s.upBuild); err != nil {
		ui.Blank()
		ui.Error("Failed to start xatu stack")

		return err
	}

	ui.Blank()
	ui.Success("Xatu stack started")
	ui.Blank()

	return printXatuStatus(ctx, runner)
}

// Down stops the xatu docker-compose stack.
func (s *xatuStack) Down(ctx context.Context) error {
	xatuCfg, _, err := config.LoadXatuConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	runner, err := newXatuRunner(s.log, xatuCfg)
	if err != nil {
		return fmt.Errorf("failed to create compose runner: %w", err)
	}

	spinner := ui.NewSpinner("Stopping xatu stack")

	if err := runner.Down(ctx, s.downVolumes, false); err != nil {
		spinner.Fail("Failed to stop xatu stack")

		return err
	}

	spinner.Success("Xatu stack stopped")

	return nil
}

// Clean removes all xatu containers, volumes, and images.
func (s *xatuStack) Clean(ctx context.Context) error {
	xatuCfg, _, err := config.LoadXatuConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	ui.Warning("WARNING: This will remove all xatu containers, volumes, and images!")
	fmt.Println("\nThis includes:")
	fmt.Println("  - All Docker containers and volumes (data will be lost)")
	fmt.Println("  - All locally built images")
	fmt.Print("\nContinue? (y/N): ")

	var response string

	_, _ = fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		ui.Info("Cancelled.")

		return nil
	}

	runner, err := newXatuRunner(s.log, xatuCfg)
	if err != nil {
		return fmt.Errorf("failed to create compose runner: %w", err)
	}

	ui.Header("Removing all xatu containers, volumes, and images...")
	ui.Blank()

	if err := runner.Down(ctx, true, true); err != nil {
		ui.Blank()
		ui.Error("Failed to clean xatu stack")

		return err
	}

	ui.Blank()
	ui.Success("Xatu workspace cleaned successfully!")

	ui.Header("Next step:")
	fmt.Println("  xcli xatu up --build     # Rebuild and start the stack")

	return nil
}

// Build builds xatu docker images.
func (s *xatuStack) Build(ctx context.Context, services []string) error {
	xatuCfg, _, err := config.LoadXatuConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	runner, err := newXatuRunner(s.log, xatuCfg)
	if err != nil {
		return fmt.Errorf("failed to create compose runner: %w", err)
	}

	ui.Header("Building xatu images...")
	ui.Blank()

	if err := runner.Build(ctx, services...); err != nil {
		ui.Blank()
		ui.Error("Build failed")

		return err
	}

	ui.Blank()
	ui.Success("Build complete")

	return nil
}

// Rebuild rebuilds and restarts a specific xatu service.
func (s *xatuStack) Rebuild(ctx context.Context, service string) error {
	xatuCfg, _, err := config.LoadXatuConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	runner, err := newXatuRunner(s.log, xatuCfg)
	if err != nil {
		return fmt.Errorf("failed to create compose runner: %w", err)
	}

	// Step 1: Build
	ui.Header(fmt.Sprintf("[1/2] Building %s...", service))
	ui.Blank()

	if err := runner.Build(ctx, service); err != nil {
		ui.Blank()
		ui.Error(fmt.Sprintf("Failed to build %s", service))

		return fmt.Errorf("failed to build service: %w", err)
	}

	ui.Blank()

	// Step 2: Restart with the new image
	ui.Header(fmt.Sprintf("[2/2] Restarting %s...", service))
	ui.Blank()

	if err := runner.Up(ctx, false, service); err != nil {
		ui.Blank()
		ui.Error(fmt.Sprintf("Failed to restart %s", service))

		return fmt.Errorf("failed to restart service: %w", err)
	}

	ui.Blank()
	ui.Success(fmt.Sprintf("%s rebuilt and restarted", service))

	return nil
}

// PrintStatus displays xatu stack status.
func (s *xatuStack) PrintStatus(ctx context.Context) error {
	xatuCfg, _, err := config.LoadXatuConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	runner, err := newXatuRunner(s.log, xatuCfg)
	if err != nil {
		return fmt.Errorf("failed to create compose runner: %w", err)
	}

	return printXatuStatus(ctx, runner)
}

// Logs shows logs for xatu services.
func (s *xatuStack) Logs(ctx context.Context, service string, follow bool) error {
	xatuCfg, _, err := config.LoadXatuConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	runner, err := newXatuRunner(s.log, xatuCfg)
	if err != nil {
		return fmt.Errorf("failed to create compose runner: %w", err)
	}

	return runner.Logs(ctx, service, follow)
}

// Start starts a specific xatu service.
func (s *xatuStack) Start(ctx context.Context, service string) error {
	xatuCfg, _, err := config.LoadXatuConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	runner, err := newXatuRunner(s.log, xatuCfg)
	if err != nil {
		return fmt.Errorf("failed to create compose runner: %w", err)
	}

	spinner := ui.NewSpinner(fmt.Sprintf("Starting %s", service))

	if err := runner.Start(ctx, service); err != nil {
		spinner.Fail(fmt.Sprintf("Failed to start %s", service))

		return fmt.Errorf("failed to start service: %w", err)
	}

	spinner.Success(fmt.Sprintf("%s started successfully", service))

	return nil
}

// Stop stops a specific xatu service.
func (s *xatuStack) Stop(ctx context.Context, service string) error {
	xatuCfg, _, err := config.LoadXatuConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	runner, err := newXatuRunner(s.log, xatuCfg)
	if err != nil {
		return fmt.Errorf("failed to create compose runner: %w", err)
	}

	spinner := ui.NewSpinner(fmt.Sprintf("Stopping %s", service))

	if err := runner.Stop(ctx, service); err != nil {
		spinner.Fail(fmt.Sprintf("Failed to stop %s", service))

		return fmt.Errorf("failed to stop service: %w", err)
	}

	spinner.Success(fmt.Sprintf("%s stopped successfully", service))

	return nil
}

// Restart restarts a specific xatu service.
func (s *xatuStack) Restart(ctx context.Context, service string) error {
	xatuCfg, _, err := config.LoadXatuConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	runner, err := newXatuRunner(s.log, xatuCfg)
	if err != nil {
		return fmt.Errorf("failed to create compose runner: %w", err)
	}

	spinner := ui.NewSpinner(fmt.Sprintf("Restarting %s", service))

	if err := runner.Restart(ctx, service); err != nil {
		spinner.Fail(fmt.Sprintf("Failed to restart %s", service))

		return fmt.Errorf("failed to restart service: %w", err)
	}

	spinner.Success(fmt.Sprintf("%s restarted successfully", service))

	return nil
}

// newXatuRunner creates a compose.Runner from xatu configuration.
func newXatuRunner(
	log logrus.FieldLogger, cfg *config.XatuConfig,
) (*compose.Runner, error) {
	return compose.NewRunner(log, cfg.Repos.Xatu, cfg.Profiles, cfg.EnvOverrides)
}

// printXatuStatus fetches and displays xatu service status as a formatted table.
func printXatuStatus(ctx context.Context, runner *compose.Runner) error {
	statuses, err := runner.PS(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service status: %w", err)
	}

	if len(statuses) == 0 {
		ui.Info("No services running")

		return nil
	}

	headers := []string{"Service", "State", "Status", "Ports"}
	rows := make([][]string, 0, len(statuses))

	for _, st := range statuses {
		state := st.State
		if state == "running" {
			state = pterm.Green(state)
		} else {
			state = pterm.Red(state)
		}

		name := st.Service
		if name == "" {
			name = st.Name
		}

		rows = append(rows, []string{name, state, st.Status, st.Ports})
	}

	ui.Table(headers, rows)

	return nil
}

// checkPortConflicts warns about potential port conflicts between xatu and lab.
func (s *xatuStack) checkPortConflicts() {
	result, err := config.Load(s.configPath)
	if err != nil || result.Config.Lab == nil || result.Config.Xatu == nil {
		return
	}

	ui.Blank()
	ui.Warning("Both lab and xatu stacks are configured. Common port conflicts:")
	fmt.Println("  - Grafana (3000), Prometheus (9090), ClickHouse (8123)")
	ui.Info("Use xatu.envOverrides in .xcli.yaml to remap xatu ports, e.g.:")
	fmt.Println("  xatu:")
	fmt.Println("    envOverrides:")
	fmt.Println("      GRAFANA_PORT: \"3001\"")
}
