package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/builder"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/configgen"
	"github.com/ethpandaops/xcli/pkg/infrastructure"
	"github.com/ethpandaops/xcli/pkg/process"
	"github.com/sirupsen/logrus"
)

// Orchestrator manages the complete lab stack
type Orchestrator struct {
	log      logrus.FieldLogger
	cfg      *config.Config
	infra    *infrastructure.Manager
	proc     *process.Manager
	builder  *builder.Manager
	stateDir string
	verbose  bool
}

// NewOrchestrator creates a new Orchestrator instance
func NewOrchestrator(log logrus.FieldLogger, cfg *config.Config) *Orchestrator {
	stateDir := ".xcli"
	return &Orchestrator{
		log:      log.WithField("component", "orchestrator"),
		cfg:      cfg,
		infra:    infrastructure.NewManager(log, cfg),
		proc:     process.NewManager(log, stateDir),
		builder:  builder.NewManager(log, cfg),
		stateDir: stateDir,
		verbose:  false,
	}
}

// SetVerbose sets verbose mode for build/setup command output
func (o *Orchestrator) SetVerbose(verbose bool) {
	o.verbose = verbose
	o.builder.SetVerbose(verbose)
	o.infra.SetVerbose(verbose)
}

// Up starts the complete stack
func (o *Orchestrator) Up(ctx context.Context, skipBuild bool, forceBuild bool) error {
	o.log.Info("starting lab stack")

	// Phase 0: Build bootstrap tooling (xatu-cbt) and other non-proto services
	if !skipBuild {
		o.log.Info("building repositories")
		if err := o.builder.BuildAll(ctx, forceBuild); err != nil {
			return fmt.Errorf("failed to build repositories: %w", err)
		}
	} else {
		// Check if required binaries exist
		status := o.builder.CheckBinariesExist()
		for name, exists := range status {
			if !exists {
				return fmt.Errorf("binary %s not found - please build first or run without --no-build", name)
			}
		}
	}

	// Phase 1: Start infrastructure
	o.log.Info("starting infrastructure")
	if err := o.infra.Start(ctx); err != nil {
		return fmt.Errorf("failed to start infrastructure: %w", err)
	}

	// Phase 2: Setup networks (run migrations)
	for _, network := range o.cfg.EnabledNetworks() {
		if err := o.infra.SetupNetwork(ctx, network.Name); err != nil {
			o.log.WithError(err).Warnf("Failed to setup network %s (may already be setup)", network.Name)
		}
	}

	// Phase 3: Generate configs (needed for proto generation)
	o.log.Info("generating service configurations")
	if err := o.generateConfigs(); err != nil {
		return fmt.Errorf("failed to generate configs: %w", err)
	}

	// Generate protos
	if !skipBuild {
		if err := o.builder.GenerateProtos(ctx); err != nil {
			return fmt.Errorf("failed to generate protos: %w", err)
		}

		// Build cbt-api
		if err := o.builder.BuildCBTAPI(ctx, forceBuild); err != nil {
			return fmt.Errorf("failed to build cbt-api: %w", err)
		}
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

// Down stops all services (keeps infrastructure running)
func (o *Orchestrator) Down(ctx context.Context) error {
	o.log.Info("stopping services")

	if err := o.proc.StopAll(); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	o.log.Info("services stopped")
	return nil
}

// Clean stops everything and removes data
func (o *Orchestrator) Clean(ctx context.Context) error {
	o.log.Info("cleaning up")

	// Stop services first
	if err := o.proc.StopAll(); err != nil {
		o.log.WithError(err).Warn("Failed to stop services")
	}

	// Reset infrastructure
	if err := o.infra.Reset(ctx); err != nil {
		return fmt.Errorf("failed to reset infrastructure: %w", err)
	}

	o.log.Info("cleanup complete")
	return nil
}

// Restart restarts a specific service
func (o *Orchestrator) Restart(ctx context.Context, service string) error {
	return o.proc.Restart(ctx, service)
}

// Logs shows logs for a service
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

// Status shows service status
func (o *Orchestrator) Status(ctx context.Context) error {
	fmt.Println("Infrastructure:")
	infraStatus := o.infra.Status()
	for name, running := range infraStatus {
		status := "✗ down"
		if running {
			status = "✓ running"
		}
		fmt.Printf("  %-20s %s\n", name, status)
	}

	fmt.Println("\nServices:")
	processes := o.proc.List()
	if len(processes) == 0 {
		fmt.Println("  No services running")
	} else {
		for _, p := range processes {
			fmt.Printf("  %-30s ✓ running (PID %d)\n", p.Name, p.PID)
		}
	}

	return nil
}

// generateConfigs generates configuration files for all services
func (o *Orchestrator) generateConfigs() error {
	configsDir := filepath.Join(o.stateDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		return fmt.Errorf("failed to create configs directory: %w", err)
	}

	generator := configgen.NewGenerator(o.log, o.cfg)

	// Generate configs for each network
	for _, network := range o.cfg.EnabledNetworks() {
		// CBT config
		cbtConfig, err := generator.GenerateCBTConfig(network.Name)
		if err != nil {
			return fmt.Errorf("failed to generate CBT config for %s: %w", network.Name, err)
		}
		cbtPath := filepath.Join(configsDir, fmt.Sprintf("cbt-%s.yaml", network.Name))
		if err := os.WriteFile(cbtPath, []byte(cbtConfig), 0644); err != nil {
			return fmt.Errorf("failed to write CBT config: %w", err)
		}

		// cbt-api config
		apiConfig, err := generator.GenerateCBTAPIConfig(network.Name)
		if err != nil {
			return fmt.Errorf("failed to generate cbt-api config for %s: %w", network.Name, err)
		}
		apiPath := filepath.Join(configsDir, fmt.Sprintf("cbt-api-%s.yaml", network.Name))
		if err := os.WriteFile(apiPath, []byte(apiConfig), 0644); err != nil {
			return fmt.Errorf("failed to write cbt-api config: %w", err)
		}
	}

	// lab-backend config
	backendConfig, err := generator.GenerateLabBackendConfig()
	if err != nil {
		return fmt.Errorf("failed to generate lab-backend config: %w", err)
	}
	backendPath := filepath.Join(configsDir, "lab-backend.yaml")
	if err := os.WriteFile(backendPath, []byte(backendConfig), 0644); err != nil {
		return fmt.Errorf("failed to write lab-backend config: %w", err)
	}

	o.log.Info("service configurations generated")
	return nil
}

// startServices starts all service processes
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

// startCBTEngine starts a CBT engine for a network
func (o *Orchestrator) startCBTEngine(ctx context.Context, network string) error {
	cbtBinary := filepath.Join(o.cfg.Repos.CBT, "bin", "cbt")
	if _, err := os.Stat(cbtBinary); os.IsNotExist(err) {
		return fmt.Errorf("cbt binary not found at %s - please run 'make build' in cbt repo", cbtBinary)
	}

	configPath, err := filepath.Abs(filepath.Join(o.stateDir, "configs", fmt.Sprintf("cbt-%s.yaml", network)))
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, cbtBinary, "--config", configPath)
	cmd.Dir = o.cfg.Repos.CBT
	cmd.Env = append(os.Environ(), fmt.Sprintf("NETWORK=%s", network))

	return o.proc.Start(ctx, fmt.Sprintf("cbt-%s", network), cmd)
}

// startCBTAPI starts cbt-api for a network
func (o *Orchestrator) startCBTAPI(ctx context.Context, network string) error {
	apiBinary := filepath.Join(o.cfg.Repos.CBTAPI, "bin", "server")
	if _, err := os.Stat(apiBinary); os.IsNotExist(err) {
		return fmt.Errorf("cbt-api binary not found at %s - please run 'make build' in cbt-api repo", apiBinary)
	}

	configPath, err := filepath.Abs(filepath.Join(o.stateDir, "configs", fmt.Sprintf("cbt-api-%s.yaml", network)))
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, apiBinary, "--config", configPath)
	cmd.Dir = o.cfg.Repos.CBTAPI

	return o.proc.Start(ctx, fmt.Sprintf("cbt-api-%s", network), cmd)
}

// startLabBackend starts lab-backend
func (o *Orchestrator) startLabBackend(ctx context.Context) error {
	backendBinary := filepath.Join(o.cfg.Repos.LabBackend, "bin", "lab-backend")
	if _, err := os.Stat(backendBinary); os.IsNotExist(err) {
		return fmt.Errorf("lab-backend binary not found at %s - please run 'make build' in lab-backend repo", backendBinary)
	}

	configPath, err := filepath.Abs(filepath.Join(o.stateDir, "configs", "lab-backend.yaml"))
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, backendBinary, "--config", configPath)
	cmd.Dir = o.cfg.Repos.LabBackend

	return o.proc.Start(ctx, "lab-backend", cmd)
}

// startLabFrontend starts the lab frontend dev server
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

	return o.proc.Start(ctx, "lab-frontend", cmd)
}
