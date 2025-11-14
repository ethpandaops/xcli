package orchestrator

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethpandaops/xcli/pkg/builder"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/configgen"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/infrastructure"
	"github.com/ethpandaops/xcli/pkg/mode"
	"github.com/ethpandaops/xcli/pkg/portutil"
	"github.com/ethpandaops/xcli/pkg/process"
	"github.com/sirupsen/logrus"
)

// Orchestrator manages the complete lab stack.
type Orchestrator struct {
	log       logrus.FieldLogger
	cfg       *config.LabConfig
	mode      mode.Mode
	infra     *infrastructure.Manager
	proc      *process.Manager
	builder   *builder.Manager
	stateDir  string
	verbose   bool
	overrides *config.CBTOverridesConfig
}

// NewOrchestrator creates a new Orchestrator instance.
func NewOrchestrator(log logrus.FieldLogger, cfg *config.LabConfig) (*Orchestrator, error) {
	stateDir := ".xcli"

	// Load user-provided CBT overrides if they exist
	overrides, err := config.LoadCBTOverrides(constants.CBTOverridesFile)
	if err != nil {
		log.WithError(err).Warn("failed to load user CBT overrides, using defaults")

		overrides = config.DefaultCBTOverrides()
	}

	// Create mode from config (wrapping LabConfig to get Config)
	fullConfig := &config.Config{Lab: cfg}

	m, err := mode.NewMode(fullConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create mode: %w", err)
	}

	// Validate mode-specific config requirements
	if err := m.ValidateConfig(fullConfig); err != nil {
		return nil, fmt.Errorf("mode validation failed: %w", err)
	}

	log.WithField("mode", m.Name()).Info("initialized orchestrator")

	return &Orchestrator{
		log:       log.WithField("component", "orchestrator"),
		cfg:       cfg,
		mode:      m,
		infra:     infrastructure.NewManager(log, cfg, m),
		proc:      process.NewManager(log, stateDir),
		builder:   builder.NewManager(log, cfg),
		stateDir:  stateDir,
		verbose:   false,
		overrides: overrides,
	}, nil
}

// SetVerbose sets verbose mode for build/setup command output.
func (o *Orchestrator) SetVerbose(verbose bool) {
	o.verbose = verbose
	o.builder.SetVerbose(verbose)
	o.infra.SetVerbose(verbose)
}

// Builder returns the build manager for direct access to build methods.
// Used by rebuild command to access individual build functions.
func (o *Orchestrator) Builder() *builder.Manager {
	return o.builder
}

// Up starts the complete stack.
func (o *Orchestrator) Up(ctx context.Context, skipBuild bool, forceBuild bool) error {
	o.log.Info("starting lab stack")

	// Fast prerequisite validation (read-only checks, no fixing)
	// Fails fast with helpful error if prerequisites not satisfied
	if err := o.validatePrerequisites(ctx); err != nil {
		return fmt.Errorf("prerequisites not satisfied: %w\n\nRun 'xcli lab init' to satisfy prerequisites", err)
	}

	// Test external ClickHouse connection early (before builds and infrastructure)
	if o.mode.NeedsExternalClickHouse() {
		if o.cfg.Infrastructure.ClickHouse.Xatu.ExternalURL == "" {
			return fmt.Errorf("external ClickHouse URL is required when using hybrid mode")
		}

		o.log.WithField("url", o.cfg.Infrastructure.ClickHouse.Xatu.ExternalURL).Info("testing external ClickHouse connection")

		if err := o.infra.TestExternalConnection(ctx); err != nil {
			return fmt.Errorf("failed to connect to external ClickHouse: %w", err)
		}

		o.log.Info("external ClickHouse connection verified")
	}

	// Check if stack is already running
	runningProcesses := o.proc.List()
	portConflicts := o.checkPortConflicts()

	if len(runningProcesses) > 0 || len(portConflicts) > 0 {
		fmt.Println("\n⚠ Stack is already running (or ports are in use)")

		if len(runningProcesses) > 0 {
			fmt.Println("\nRunning services:")

			for _, p := range runningProcesses {
				fmt.Printf("  - %s (PID %d)\n", p.Name, p.PID)
			}
		}

		if len(portConflicts) > 0 {
			fmt.Println("\nPort conflicts detected:")
			fmt.Print(portutil.FormatConflicts(portConflicts))
		}

		return fmt.Errorf("stack is already running")
	}

	// Phase 0: Build xatu-cbt (ONCE, not in Phase 2)
	// Note: xatu-cbt is built separately here because infrastructure needs it before Phase 2
	// Reason: Infrastructure startup requires xatu-cbt binary to run migrations and services
	// This ensures xatu-cbt is ready before starting infrastructure in Phase 1
	if !skipBuild {
		o.log.Info("building xatu-cbt (required for infrastructure)")

		if err := o.builder.BuildXatuCBT(ctx, forceBuild); err != nil {
			return fmt.Errorf("failed to build xatu-cbt: %w", err)
		}

		o.log.Info("xatu-cbt built successfully")
	} else {
		// Check if xatu-cbt binary exists
		if !o.builder.XatuCBTBinaryExists() {
			return fmt.Errorf("xatu-cbt binary not found - please build first or run without --no-build")
		}
	}

	// Phase 1: Start infrastructure
	o.log.WithField("mode", o.mode.Name()).Info("starting infrastructure")

	if err := o.infra.Start(ctx); err != nil {
		return fmt.Errorf("failed to start infrastructure: %w", err)
	}

	o.log.WithField("mode", o.mode.Name()).Info("infrastructure ready")

	// Phase 2: Build all repositories (parallel, excluding xatu-cbt)
	// Note: xatu-cbt already built in Phase 0
	// BuildAll now runs: CBT || lab-backend || lab (parallel execution)
	if !skipBuild {
		o.log.Info("building repositories (parallel)")

		if err := o.builder.BuildAll(ctx, forceBuild); err != nil {
			return fmt.Errorf("failed to build repositories: %w", err)
		}

		o.log.Info("all repositories built successfully")
	} else {
		// Check if required binaries exist
		status := o.builder.CheckBinariesExist()
		for name, exists := range status {
			if !exists {
				return fmt.Errorf("binary %s not found - please build first or run without --no-build", name)
			}
		}
	}

	// Phase 3: Setup networks (run migrations)
	for _, network := range o.cfg.EnabledNetworks() {
		if err := o.infra.SetupNetwork(ctx, network.Name); err != nil {
			o.log.WithError(err).Warnf("Failed to setup network %s (may already be setup)", network.Name)
		}
	}

	// Phase 4: Generate configs (needed for proto generation)
	o.log.Info("generating service configurations")

	if err := o.GenerateConfigs(); err != nil {
		return fmt.Errorf("failed to generate configs: %w", err)
	}

	// Build cbt-api (includes proto generation in its DAG)
	if !skipBuild {
		if err := o.builder.BuildCBTAPI(ctx, forceBuild); err != nil {
			return fmt.Errorf("failed to build cbt-api: %w", err)
		}
	}

	// Check for port conflicts before starting services
	o.log.Info("checking for port conflicts")

	if conflicts := o.checkPortConflicts(); len(conflicts) > 0 {
		fmt.Println("\n⚠ Port conflicts detected!")
		fmt.Print(portutil.FormatConflicts(conflicts))

		return fmt.Errorf("port conflicts prevent starting services")
	}

	// Start services
	if err := o.startServices(ctx); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	fmt.Println("\n✓ Stack is running!")
	fmt.Println("\nServices:")
	fmt.Printf("  Lab Frontend:  http://localhost:%d\n", o.cfg.Ports.LabFrontend)
	fmt.Printf("  Lab Backend:   http://localhost:%d\n", o.cfg.Ports.LabBackend)

	for _, net := range o.cfg.EnabledNetworks() {
		fmt.Printf("  CBT API (%s): http://localhost:%d\n", net.Name, o.cfg.GetCBTAPIPort(net.Name))
	}

	fmt.Println()

	return nil
}

// Down stops all services and tears down infrastructure (removes volumes).
func (o *Orchestrator) Down(ctx context.Context) error {
	o.log.Info("tearing down stack")

	// Stop services first
	o.log.Info("stopping services")

	if err := o.proc.StopAll(); err != nil {
		o.log.WithError(err).Warn("failed to stop services")
	}

	// Check for and clean up orphaned processes
	o.log.Info("checking for orphaned processes")

	orphanedCount := o.cleanupOrphanedProcesses()
	if orphanedCount > 0 {
		o.log.WithField("count", orphanedCount).Info("cleaned up orphaned processes")
	}

	// Clean log files
	o.log.Info("cleaning log files")

	if err := o.proc.CleanLogs(); err != nil {
		o.log.WithError(err).Warn("failed to clean logs")
	}

	// Reset infrastructure (stops containers and removes volumes)
	o.log.Info("resetting infrastructure")

	if err := o.infra.Reset(ctx); err != nil {
		return fmt.Errorf("failed to reset infrastructure: %w", err)
	}

	o.log.Info("teardown complete")
	fmt.Println("\n✓ Stack torn down successfully")
	fmt.Println("\nAll services stopped, logs cleaned, and volumes removed.")
	fmt.Println("Run 'xcli lab up' to start fresh.")

	return nil
}

// StopServices stops all running services without tearing down infrastructure.
func (o *Orchestrator) StopServices() error {
	o.log.Info("stopping all services")

	if err := o.proc.StopAll(); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	return nil
}

// Restart restarts a specific service.
func (o *Orchestrator) Restart(ctx context.Context, service string) error {
	// Stop the service
	if err := o.StopService(ctx, service); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	o.log.WithField("service", service).Info("service stopped, starting again")

	// Start it again
	return o.StartService(ctx, service)
}

// StartService starts a specific service by name.
func (o *Orchestrator) StartService(ctx context.Context, service string) error {
	// Use background context for long-running processes
	processCtx := context.Background()

	// Parse service name to determine type and network
	switch {
	case service == constants.ServiceLabBackend:
		return o.startLabBackend(processCtx)
	case service == constants.ServiceLabFrontend:
		return o.startLabFrontend(processCtx)
	case strings.HasPrefix(service, constants.ServicePrefixCBTAPI):
		network := strings.TrimPrefix(service, constants.ServicePrefixCBTAPI)
		// Validate network is enabled
		if !o.isNetworkEnabled(network) {
			return fmt.Errorf("network %s is not enabled in config", network)
		}

		return o.startCBTAPI(processCtx, network)
	case strings.HasPrefix(service, constants.ServicePrefixCBT):
		network := strings.TrimPrefix(service, constants.ServicePrefixCBT)
		// Validate network is enabled
		if !o.isNetworkEnabled(network) {
			return fmt.Errorf("network %s is not enabled in config", network)
		}

		return o.startCBTEngine(processCtx, network)
	default:
		return fmt.Errorf("unknown service: %s", service)
	}
}

// StopService stops a specific service by name.
func (o *Orchestrator) StopService(ctx context.Context, service string) error {
	return o.proc.Stop(service)
}

// isNetworkEnabled checks if a network is enabled in the configuration.
func (o *Orchestrator) isNetworkEnabled(network string) bool {
	for _, net := range o.cfg.EnabledNetworks() {
		if net.Name == network {
			return true
		}
	}

	return false
}

// Logs shows logs for a service.
func (o *Orchestrator) Logs(ctx context.Context, service string, follow bool) error {
	if service == "" {
		// Show all logs
		processes := o.proc.List()
		for _, p := range processes {
			fmt.Printf("==> %s <==\n", p.Name)

			if err := o.proc.TailLogs(ctx, p.Name, false); err != nil {
				o.log.WithError(err).Warnf("Failed to read logs for %s", p.Name)
			}

			fmt.Println()
		}

		return nil
	}

	return o.proc.TailLogs(ctx, service, follow)
}

// Status shows service status.
func (o *Orchestrator) Status(ctx context.Context) error {
	// Show current mode
	fmt.Printf("Mode: %s\n\n", o.mode.Name())

	// Show infrastructure status
	fmt.Println("Infrastructure:")

	infraStatus := o.infra.Status()

	// Use mode interface to determine if external ClickHouse is used
	needsExternal := o.mode.NeedsExternalClickHouse()

	for name, running := range infraStatus {
		// Skip showing Xatu ClickHouse status in hybrid mode (it's external)
		if needsExternal && name == "ClickHouse Xatu" {
			continue
		}

		status := "✗ down"
		if running {
			status = "✓ running"
		}

		fmt.Printf("  %-20s %s\n", name, status)
	}

	// In hybrid mode, show external Xatu connection info
	if needsExternal {
		externalInfo := o.sanitizeURL(o.cfg.Infrastructure.ClickHouse.Xatu.ExternalURL)
		fmt.Printf("  %-20s ↗ external (%s)\n",
			"ClickHouse Xatu",
			externalInfo,
		)
	}

	// Show services
	fmt.Println("\nServices:")

	processes := o.proc.List()
	if len(processes) == 0 {
		fmt.Println("  No services running")
	} else {
		for _, p := range processes {
			fmt.Printf("  %-30s ✓ running (PID %d)\n", p.Name, p.PID)
		}
	}

	// Check for orphaned processes
	conflicts := o.checkPortConflicts()
	if len(conflicts) > 0 && len(processes) == 0 {
		// Only show orphan warning if no managed processes are running
		fmt.Println("\n⚠ Warning: Orphaned processes detected on lab ports")
		fmt.Println("These processes may be from a previous xcli session:")

		for _, c := range conflicts {
			if c.PID > 0 {
				processInfo := fmt.Sprintf("PID %d", c.PID)

				if c.Process != "" {
					processInfo = fmt.Sprintf("%s (%s)", processInfo, c.Process)
				}

				fmt.Printf("  Port %d: %s\n", c.Port, processInfo)
			}
		}

		fmt.Println("\nRun 'xcli lab down' to clean up orphaned processes")
	}

	return nil
}

// validatePrerequisites performs fast read-only checks (no auto-fixing).
// Validates that required files/directories exist without running expensive operations.
// Users must run 'xcli lab init' to satisfy prerequisites.
func (o *Orchestrator) validatePrerequisites(ctx context.Context) error {
	o.log.Info("validating prerequisites (fast check)")

	// Check that all required repositories exist
	requiredRepos := map[string]string{
		"cbt":         o.cfg.Repos.CBT,
		"xatu-cbt":    o.cfg.Repos.XatuCBT,
		"cbt-api":     o.cfg.Repos.CBTAPI,
		"lab-backend": o.cfg.Repos.LabBackend,
		"lab":         o.cfg.Repos.Lab,
	}

	for repoName, repoPath := range requiredRepos {
		if repoPath == "" {
			return fmt.Errorf("repository %s not configured", repoName)
		}

		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			return fmt.Errorf("repository %s not found at %s", repoName, repoPath)
		}
	}

	// Check that critical files exist (quick validation)
	criticalPaths := map[string]string{
		"lab node_modules":   filepath.Join(o.cfg.Repos.Lab, "node_modules"),
		"lab-backend .env":   filepath.Join(o.cfg.Repos.LabBackend, ".env"),
		"lab-backend go.mod": filepath.Join(o.cfg.Repos.LabBackend, "go.mod"),
		"cbt go.mod":         filepath.Join(o.cfg.Repos.CBT, "go.mod"),
		"xatu-cbt Makefile":  filepath.Join(o.cfg.Repos.XatuCBT, "Makefile"),
		"xatu-cbt docker":    filepath.Join(o.cfg.Repos.XatuCBT, "docker-compose.platform.yml"),
	}

	for name, path := range criticalPaths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("required file/directory missing: %s (at %s)", name, path)
		}
	}

	o.log.Info("prerequisites validation passed")

	return nil
}

// checkPortConflicts checks if any ports needed by services are already in use.
func (o *Orchestrator) checkPortConflicts() []portutil.PortConflict {
	portsToCheck := make([]int, 0)
	enabledNetworks := o.cfg.EnabledNetworks()

	// Lab backend and frontend ports
	portsToCheck = append(portsToCheck, o.cfg.Ports.LabBackend)
	portsToCheck = append(portsToCheck, o.cfg.Ports.LabFrontend)

	// CBT and CBT API ports for each enabled network
	for i, network := range enabledNetworks {
		// CBT API service port
		portsToCheck = append(portsToCheck, o.cfg.GetCBTAPIPort(network.Name))

		// CBT metrics port (9100 + network index)
		portsToCheck = append(portsToCheck, 9100+i)

		// CBT API metrics port (9200 + network index)
		portsToCheck = append(portsToCheck, 9200+i)
	}

	return portutil.CheckPorts(portsToCheck)
}

// cleanupOrphanedProcesses finds and kills orphaned lab processes on expected ports.
// Returns the number of processes killed.
func (o *Orchestrator) cleanupOrphanedProcesses() int {
	conflicts := o.checkPortConflicts()
	if len(conflicts) == 0 {
		return 0
	}

	killedCount := 0

	for _, conflict := range conflicts {
		if conflict.PID > 0 {
			o.log.WithFields(logrus.Fields{
				"pid":     conflict.PID,
				"port":    conflict.Port,
				"process": conflict.Process,
			}).Info("killing orphaned process")

			if err := portutil.KillProcess(conflict.PID); err != nil {
				o.log.WithError(err).Warnf("failed to kill process %d", conflict.PID)
			} else {
				killedCount++
			}
		}
	}

	return killedCount
}

// sanitizeURL removes credentials from a URL for display purposes.
func (o *Orchestrator) sanitizeURL(rawURL string) string {
	if rawURL == "" {
		return "not configured"
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		// If parsing fails, just show the host part or return as-is
		return rawURL
	}

	// Remove password if present
	if parsedURL.User != nil {
		username := parsedURL.User.Username()
		if username != "" {
			parsedURL.User = url.User(username) // Keep username, remove password
		}
	}

	return parsedURL.String()
}

// hasCustomConfig checks if a custom config exists in the custom-configs directory.
// Returns true and the path if found, false otherwise.
func (o *Orchestrator) hasCustomConfig(filename string) (bool, string) {
	customPath := filepath.Join(o.stateDir, "custom-configs", filename)
	if _, err := os.Stat(customPath); err == nil {
		return true, customPath
	}

	return false, ""
}

// copyCustomConfig copies a custom config file to the configs directory.
func (o *Orchestrator) copyCustomConfig(customPath, destPath string) error {
	data, err := os.ReadFile(customPath)
	if err != nil {
		return fmt.Errorf("failed to read custom config: %w", err)
	}

	//nolint:gosec // Config file permissions are intentionally 0644 for readability
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write custom config: %w", err)
	}

	return nil
}

// GenerateConfigs generates configuration files for all services.
// Public method so it can be called by rebuild commands.
func (o *Orchestrator) GenerateConfigs() error {
	configsDir := filepath.Join(o.stateDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		return fmt.Errorf("failed to create configs directory: %w", err)
	}

	generator := configgen.NewGenerator(o.log, o.cfg, o.overrides)

	// Generate configs for each network
	for _, network := range o.cfg.EnabledNetworks() {
		// Generate CBT overrides first (if any)
		overridesFilename := fmt.Sprintf(constants.ConfigFileCBTOverride, network.Name)
		overridesPath := filepath.Join(configsDir, overridesFilename)

		overridesContent, err := generator.GenerateCBTOverrides(network.Name)
		if err != nil {
			return fmt.Errorf("failed to generate CBT overrides for %s: %w", network.Name, err)
		}

		//nolint:gosec // Config file permissions are intentionally 0644 for readability
		err = os.WriteFile(overridesPath, []byte(overridesContent), 0644)
		if err != nil {
			return fmt.Errorf("failed to write CBT overrides: %w", err)
		}

		slotPos, timestampPos := config.CalculateTwoWeeksAgoPosition(network.Name, network.GenesisTimestamp)
		o.log.WithFields(logrus.Fields{
			"network":      network.Name,
			"slot_limit":   slotPos,
			"ts_limit":     timestampPos,
			"duration":     o.cfg.CBT.DefaultBackfillDuration,
			"user_config":  o.overrides != nil && len(o.overrides.Models) > 0,
			"has_defaults": o.overrides == nil || o.overrides.DefaultLimits == nil,
		}).Info("generated CBT model overrides with backfill limit")

		// CBT config
		cbtFilename := fmt.Sprintf(constants.ConfigFileCBT, network.Name)
		cbtPath := filepath.Join(configsDir, cbtFilename)

		if hasCustom, customPath := o.hasCustomConfig(cbtFilename); hasCustom {
			o.log.WithField("network", network.Name).Info("using custom CBT config")

			if err := o.copyCustomConfig(customPath, cbtPath); err != nil {
				return fmt.Errorf("failed to copy custom CBT config for %s: %w", network.Name, err)
			}
		} else {
			cbtConfig, err := generator.GenerateCBTConfig(network.Name, overridesPath)
			if err != nil {
				return fmt.Errorf("failed to generate CBT config for %s: %w", network.Name, err)
			}

			//nolint:gosec // Config file permissions are intentionally 0644 for readability
			err = os.WriteFile(cbtPath, []byte(cbtConfig), 0644)
			if err != nil {
				return fmt.Errorf("failed to write CBT config: %w", err)
			}
		}

		// cbt-api config
		apiFilename := fmt.Sprintf(constants.ConfigFileCBTAPI, network.Name)
		apiPath := filepath.Join(configsDir, apiFilename)

		if hasCustom, customPath := o.hasCustomConfig(apiFilename); hasCustom {
			o.log.WithField("network", network.Name).Info("using custom cbt-api config")

			if err := o.copyCustomConfig(customPath, apiPath); err != nil {
				return fmt.Errorf("failed to copy custom cbt-api config for %s: %w", network.Name, err)
			}
		} else {
			apiConfig, apiErr := generator.GenerateCBTAPIConfig(network.Name)
			if apiErr != nil {
				return fmt.Errorf("failed to generate cbt-api config for %s: %w", network.Name, apiErr)
			}

			//nolint:gosec // Config file permissions are intentionally 0644 for readability
			if apiErr := os.WriteFile(apiPath, []byte(apiConfig), 0644); apiErr != nil {
				return fmt.Errorf("failed to write cbt-api config: %w", apiErr)
			}
		}
	}

	// lab-backend config
	backendFilename := constants.ConfigFileLabBackend
	backendPath := filepath.Join(configsDir, backendFilename)

	if hasCustom, customPath := o.hasCustomConfig(backendFilename); hasCustom {
		o.log.Info("using custom lab-backend config")

		if err := o.copyCustomConfig(customPath, backendPath); err != nil {
			return fmt.Errorf("failed to copy custom lab-backend config: %w", err)
		}
	} else {
		backendConfig, err := generator.GenerateLabBackendConfig()
		if err != nil {
			return fmt.Errorf("failed to generate lab-backend config: %w", err)
		}

		//nolint:gosec // Config file permissions are intentionally 0644 for readability
		if err := os.WriteFile(backendPath, []byte(backendConfig), 0644); err != nil {
			return fmt.Errorf("failed to write lab-backend config: %w", err)
		}
	}

	o.log.Info("service configurations generated")

	return nil
}

// startServices starts all service processes.
func (o *Orchestrator) startServices(ctx context.Context) error {
	o.log.Info("starting services")

	// Use background context for long-running processes
	// The parent context is only for the startup phase
	processCtx := context.Background()

	// Start CBT engines for each network
	for _, network := range o.cfg.EnabledNetworks() {
		if err := o.startCBTEngine(processCtx, network.Name); err != nil {
			return fmt.Errorf("failed to start CBT engine for %s: %w", network.Name, err)
		}
	}

	// Start cbt-api for each network
	for _, network := range o.cfg.EnabledNetworks() {
		if err := o.startCBTAPI(processCtx, network.Name); err != nil {
			return fmt.Errorf("failed to start cbt-api for %s: %w", network.Name, err)
		}
	}

	// Start lab-backend
	if err := o.startLabBackend(processCtx); err != nil {
		return fmt.Errorf("failed to start lab-backend: %w", err)
	}

	// Start lab frontend
	if err := o.startLabFrontend(processCtx); err != nil {
		return fmt.Errorf("failed to start lab frontend: %w", err)
	}

	return nil
}

// AreServicesRunning checks if CBT and cbt-api services are currently running.
// Returns true if at least one service is running (safe to restart).
func (o *Orchestrator) AreServicesRunning() bool {
	for _, network := range o.cfg.EnabledNetworks() {
		cbtService := constants.ServiceNameCBT(network.Name)
		cbtAPIService := constants.ServiceNameCBTAPI(network.Name)

		if _, exists := o.proc.Get(cbtService); exists {
			return true
		}

		if _, exists := o.proc.Get(cbtAPIService); exists {
			return true
		}
	}

	return false
}

// RestartServices restarts cbt-api and CBT engines without full stack restart.
// Used by 'xcli lab rebuild xatu-cbt' to apply proto/config changes.
// Leverages existing process manager (o.proc) for service lifecycle management.
func (o *Orchestrator) RestartServices(ctx context.Context, verbose bool) error {
	if verbose {
		fmt.Println("Restarting services (cbt-api + CBT engines)...")
	}

	// Get list of services to restart (cbt-api and CBT engines only)
	// Service names follow patterns: "cbt-<network>", "cbt-api-<network>"
	enabledNetworks := o.cfg.EnabledNetworks()
	servicesToRestart := make([]string, 0, 2*len(enabledNetworks))

	// Collect CBT engine service names for all enabled networks
	for _, network := range enabledNetworks {
		servicesToRestart = append(servicesToRestart, constants.ServiceNameCBT(network.Name))
	}

	// Collect cbt-api service names for all enabled networks
	for _, network := range enabledNetworks {
		servicesToRestart = append(servicesToRestart, constants.ServiceNameCBTAPI(network.Name))
	}

	if verbose {
		fmt.Printf("Services to restart: %v\n", servicesToRestart)
	}

	// Stop all target services first
	for _, serviceName := range servicesToRestart {
		if verbose {
			fmt.Printf("Stopping %s...\n", serviceName)
		}

		// Use existing process manager Stop method
		if err := o.proc.Stop(serviceName); err != nil {
			// Log warning but continue - service might not be running
			if verbose {
				fmt.Printf("Warning: Failed to stop %s: %v\n", serviceName, err)
			}
		}
	}

	// Wait for cleanup
	time.Sleep(500 * time.Millisecond)

	// Use background context for long-running processes
	processCtx := context.Background()

	// Restart services in proper order
	// 1. CBT engines first
	for _, network := range o.cfg.EnabledNetworks() {
		if verbose {
			fmt.Printf("Starting CBT engine for %s...\n", network.Name)
		}

		if err := o.startCBTEngine(processCtx, network.Name); err != nil {
			return fmt.Errorf("failed to restart CBT engine for %s: %w", network.Name, err)
		}
	}

	// 2. cbt-api second
	for _, network := range o.cfg.EnabledNetworks() {
		if verbose {
			fmt.Printf("Starting cbt-api for %s...\n", network.Name)
		}

		if err := o.startCBTAPI(processCtx, network.Name); err != nil {
			return fmt.Errorf("failed to restart cbt-api for %s: %w", network.Name, err)
		}
	}

	fmt.Println("✓ Services restarted successfully")

	return nil
}

// startCBTEngine starts a CBT engine for a network.
func (o *Orchestrator) startCBTEngine(ctx context.Context, network string) error {
	cbtBinary := filepath.Join(o.cfg.Repos.CBT, constants.DirBin, constants.BinaryCBT)
	if _, err := os.Stat(cbtBinary); os.IsNotExist(err) {
		return fmt.Errorf("cbt binary not found at %s - please run 'make build' in cbt repo", cbtBinary)
	}

	configPath, err := filepath.Abs(filepath.Join(o.stateDir, constants.DirConfigs, fmt.Sprintf(constants.ConfigFileCBT, network)))
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, cbtBinary, "--config", configPath)
	cmd.Dir = o.cfg.Repos.CBT

	cmd.Env = append(os.Environ(), fmt.Sprintf("NETWORK=%s", network))

	return o.proc.Start(ctx, constants.ServiceNameCBT(network), cmd, nil)
}

// startCBTAPI starts cbt-api for a network.
func (o *Orchestrator) startCBTAPI(ctx context.Context, network string) error {
	apiBinary := filepath.Join(o.cfg.Repos.CBTAPI, constants.DirBin, constants.BinaryCBTAPI)
	if _, err := os.Stat(apiBinary); os.IsNotExist(err) {
		return fmt.Errorf("cbt-api binary not found at %s - please run 'make build' in cbt-api repo", apiBinary)
	}

	configPath, err := filepath.Abs(filepath.Join(o.stateDir, constants.DirConfigs, fmt.Sprintf(constants.ConfigFileCBTAPI, network)))
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, apiBinary, "--config", configPath)
	cmd.Dir = o.cfg.Repos.CBTAPI

	return o.proc.Start(ctx, constants.ServiceNameCBTAPI(network), cmd, nil)
}

// startLabBackend starts lab-backend.
func (o *Orchestrator) startLabBackend(ctx context.Context) error {
	backendBinary := filepath.Join(o.cfg.Repos.LabBackend, constants.DirBin, constants.BinaryLabBackend)
	if _, err := os.Stat(backendBinary); os.IsNotExist(err) {
		return fmt.Errorf("lab-backend binary not found at %s - please run 'make build' in lab-backend repo", backendBinary)
	}

	configPath, err := filepath.Abs(filepath.Join(o.stateDir, constants.DirConfigs, constants.ConfigFileLabBackend))
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, backendBinary, "--config", configPath)
	cmd.Dir = o.cfg.Repos.LabBackend

	return o.proc.Start(ctx, constants.ServiceLabBackend, cmd, nil)
}

// startLabFrontend starts the lab frontend dev server.
func (o *Orchestrator) startLabFrontend(ctx context.Context) error {
	// Check if pnpm is available
	if _, err := exec.LookPath("pnpm"); err != nil {
		return fmt.Errorf("pnpm not found - please install pnpm")
	}

	// Check if node_modules exists
	nodeModules := filepath.Join(o.cfg.Repos.Lab, "node_modules")
	if _, err := os.Stat(nodeModules); os.IsNotExist(err) {
		o.log.Warn("node_modules not found, running pnpm install")

		installCmd := exec.CommandContext(ctx, "pnpm", "install")
		installCmd.Dir = o.cfg.Repos.Lab
		installCmd.Stdout = os.Stdout

		installCmd.Stderr = os.Stderr
		if err := installCmd.Run(); err != nil {
			return fmt.Errorf("failed to run pnpm install: %w", err)
		}
	}

	cmd := exec.CommandContext(ctx, "pnpm", "dev")
	cmd.Dir = o.cfg.Repos.Lab
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("VITE_BACKEND_URL=http://localhost:%d", o.cfg.Ports.LabBackend),
	)

	return o.proc.Start(ctx, constants.ServiceLabFrontend, cmd, nil)
}
