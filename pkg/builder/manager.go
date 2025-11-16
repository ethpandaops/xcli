// Package builder handles building all lab repositories including
// Go binaries, frontend dependencies, and protobuf generation.
package builder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
	executil "github.com/ethpandaops/xcli/pkg/exec"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// Manager handles building all repositories.
type Manager struct {
	log     logrus.FieldLogger
	cfg     *config.LabConfig
	verbose bool
}

// NewManager creates a new build manager.
func NewManager(log logrus.FieldLogger, cfg *config.LabConfig) *Manager {
	return &Manager{
		log:     log.WithField("component", "builder"),
		cfg:     cfg,
		verbose: false,
	}
}

// SetVerbose sets verbose mode for build commands.
func (m *Manager) SetVerbose(verbose bool) {
	m.verbose = verbose
}

// BuildAll builds all repositories EXCEPT xatu-cbt (built in Phase 0).
// Runs CBT, lab-backend, and lab in parallel using errgroup.
func (m *Manager) BuildAll(ctx context.Context, force bool) error {
	m.log.Info("building all repositories")

	// Create progress bar if not in verbose mode
	var progressBar *ui.ProgressBar
	if !m.verbose {
		progressBar = ui.NewProgressBar("Building repositories", 3)
	}

	g, ctx := errgroup.WithContext(ctx)

	// Build CBT in parallel
	g.Go(func() error {
		spinner := m.startBuildSpinner("cbt")
		err := m.BuildCBT(ctx, force)
		m.finishBuildSpinner(spinner, "cbt", err, progressBar)

		return err
	})

	// Build lab-backend in parallel
	g.Go(func() error {
		spinner := m.startBuildSpinner("lab-backend")
		err := m.BuildLabBackend(ctx, force)
		m.finishBuildSpinner(spinner, "lab-backend", err, progressBar)

		return err
	})

	// Install lab dependencies in parallel
	g.Go(func() error {
		spinner := m.startBuildSpinner("lab")
		err := m.installLabDeps(ctx, force)
		m.finishBuildSpinner(spinner, "lab", err, progressBar)

		return err
	})

	// Wait for all builds to complete
	if err := g.Wait(); err != nil {
		if progressBar != nil {
			_ = progressBar.Stop()
		}

		return err
	}

	if progressBar != nil {
		_ = progressBar.Stop()
	}

	return nil
}

// startBuildSpinner creates a spinner for a build task if not in verbose mode.
func (m *Manager) startBuildSpinner(name string) *ui.Spinner {
	if m.verbose {
		return nil
	}

	return ui.NewSilentSpinner(fmt.Sprintf("Building %s", name))
}

// finishBuildSpinner updates spinner based on build result.
func (m *Manager) finishBuildSpinner(spinner *ui.Spinner, name string, err error, progressBar *ui.ProgressBar) {
	if m.verbose {
		return
	}

	if spinner == nil {
		return
	}

	if err != nil {
		spinner.Fail(fmt.Sprintf("Failed to build %s", name))

		return
	}

	_ = spinner.Stop()

	if progressBar != nil {
		progressBar.Increment()
	}
}

// BuildXatuCBT builds only the xatu-cbt binary (needed for infrastructure startup).
func (m *Manager) BuildXatuCBT(ctx context.Context, force bool) error {
	return m.buildXatuCBT(ctx, force)
}

// XatuCBTBinaryExists checks if the xatu-cbt binary exists.
func (m *Manager) XatuCBTBinaryExists() bool {
	binary := filepath.Join(m.cfg.Repos.XatuCBT, "bin", "xatu-cbt")

	return m.binaryExists(binary)
}

// buildXatuCBT builds the xatu-cbt binary (Phase 0 only, NOT in BuildAll).
func (m *Manager) buildXatuCBT(ctx context.Context, force bool) error {
	binary := filepath.Join(m.cfg.Repos.XatuCBT, "bin", "xatu-cbt")

	if !force && m.binaryExists(binary) {
		m.log.WithField("repo", "xatu-cbt").Info("binary exists, skipping build")

		return nil
	}

	m.log.WithField("repo", "xatu-cbt").Info("building project")

	return m.runMake(ctx, m.cfg.Repos.XatuCBT, "build")
}

// BuildCBT builds the cbt binary.
func (m *Manager) BuildCBT(ctx context.Context, force bool) error {
	binary := filepath.Join(m.cfg.Repos.CBT, "bin", "cbt")

	if !force && m.binaryExists(binary) {
		m.log.WithField("repo", "cbt").Info("binary exists, skipping build")

		return nil
	}

	m.log.WithField("repo", "cbt").Info("building project")

	// Build CBT frontend first (required for embedding in binary)
	if err := m.buildCBTFrontend(ctx, force); err != nil {
		return fmt.Errorf("failed to build CBT frontend: %w", err)
	}

	// CBT doesn't have a Makefile build target, build directly
	binDir := filepath.Join(m.cfg.Repos.CBT, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	cmd.Dir = m.cfg.Repos.CBT

	if err := executil.RunCmd(cmd, m.verbose); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}

	return nil
}

// buildCBTFrontend builds the CBT frontend (React/Vite app).
// The frontend is embedded into the CBT binary via go:embed, so it must be built before the Go binary.
func (m *Manager) buildCBTFrontend(ctx context.Context, force bool) error {
	frontendDir := filepath.Join(m.cfg.Repos.CBT, "frontend")
	frontendBuild := filepath.Join(frontendDir, "build", "frontend")

	// Check if frontend build already exists (skip unless force rebuild)
	if !force && m.dirExists(frontendBuild) {
		m.log.WithField("repo", "cbt/frontend").Info("frontend build exists, skipping")

		return nil
	}

	// Always install dependencies to ensure they match package.json
	m.log.WithField("repo", "cbt/frontend").Info("installing dependencies")

	cmd := exec.CommandContext(ctx, "pnpm", "install")
	cmd.Dir = frontendDir

	if err := executil.RunCmd(cmd, m.verbose); err != nil {
		return fmt.Errorf("pnpm install failed: %w", err)
	}

	// Build frontend
	m.log.WithField("repo", "cbt/frontend").Info("building frontend")

	cmd = exec.CommandContext(ctx, "pnpm", "build")
	cmd.Dir = frontendDir

	if err := executil.RunCmd(cmd, m.verbose); err != nil {
		return fmt.Errorf("pnpm build failed: %w", err)
	}

	return nil
}

// BuildCBTAPI builds cbt-api with proto generation
// Proto generation MUST happen first (explicit dependency in graph).
func (m *Manager) BuildCBTAPI(ctx context.Context, force bool) error {
	// Step 1: Generate protos
	if err := m.GenerateProtos(ctx); err != nil {
		return err
	}

	// Step 2: Build cbt-api binary
	binary := filepath.Join(m.cfg.Repos.CBTAPI, "bin", "server")

	if !force && m.binaryExists(binary) {
		m.log.WithField("repo", "cbt-api").Info("binary exists, skipping build")

		return nil
	}

	m.log.WithField("repo", "cbt-api").Info("building project")

	// Generate OpenAPI and other code (requires proto to be run first)
	if err := m.runMake(ctx, m.cfg.Repos.CBTAPI, "generate"); err != nil {
		return fmt.Errorf("make generate failed: %w", err)
	}

	// Build the binary
	return m.runMake(ctx, m.cfg.Repos.CBTAPI, "build-binary")
}

// BuildLabBackend builds lab-backend binary.
func (m *Manager) BuildLabBackend(ctx context.Context, force bool) error {
	binary := filepath.Join(m.cfg.Repos.LabBackend, "bin", "lab-backend")

	if !force && m.binaryExists(binary) {
		m.log.WithField("repo", "lab-backend").Info("binary exists, skipping build")

		return nil
	}

	m.log.WithField("repo", "lab-backend").Info("building project")

	return m.runMake(ctx, m.cfg.Repos.LabBackend, "build")
}

// BuildLabFrontend regenerates frontend API types from cbt-api OpenAPI spec.
func (m *Manager) BuildLabFrontend(ctx context.Context) error {
	m.log.WithField("repo", "lab").Info("regenerating API types from cbt-api")

	// Get the first enabled network to use for the OpenAPI endpoint
	networks := m.cfg.EnabledNetworks()
	if len(networks) == 0 {
		return fmt.Errorf("no networks enabled - cannot determine cbt-api port")
	}

	// Use the first enabled network's cbt-api port
	cbtAPIPort := m.cfg.GetCBTAPIPort(networks[0].Name)

	// Construct OpenAPI URL using cbt-api port
	openapiURL := fmt.Sprintf("http://localhost:%d/openapi.yaml", cbtAPIPort)

	cmd := exec.CommandContext(ctx, "pnpm", "run", "generate:api")
	cmd.Dir = m.cfg.Repos.Lab

	cmd.Env = append(os.Environ(), fmt.Sprintf("OPENAPI_INPUT=%s", openapiURL))

	if err := executil.RunCmd(cmd, m.verbose); err != nil {
		return fmt.Errorf("pnpm run generate:api failed: %w", err)
	}

	return nil
}

// installLabDeps installs lab frontend dependencies.
func (m *Manager) installLabDeps(ctx context.Context, force bool) error {
	nodeModules := filepath.Join(m.cfg.Repos.Lab, "node_modules")

	if !force && m.dirExists(nodeModules) {
		m.log.WithField("repo", "lab").Info("dependencies exist, skipping install")

		return nil
	}

	m.log.WithField("repo", "lab").Info("installing dependencies")

	cmd := exec.CommandContext(ctx, "pnpm", "install")
	cmd.Dir = m.cfg.Repos.Lab

	if err := executil.RunCmd(cmd, m.verbose); err != nil {
		return fmt.Errorf("pnpm install failed: %w", err)
	}

	return nil
}

// GenerateXatuCBTProtos generates protobuf files for xatu-cbt.
func (m *Manager) GenerateXatuCBTProtos(ctx context.Context) error {
	m.log.WithField("repo", "xatu-cbt").Info("generating protos")

	if err := m.runMake(ctx, m.cfg.Repos.XatuCBT, "proto"); err != nil {
		return fmt.Errorf("failed to generate xatu-cbt protos: %w", err)
	}

	return nil
}

// GenerateProtos generates protobuf files for cbt-api.
func (m *Manager) GenerateProtos(ctx context.Context) error {
	// Generate cbt-api protos (only for first network, they're network-agnostic)
	// We'll use mainnet as the source for table schemas
	network := m.cfg.EnabledNetworks()[0]

	m.log.WithFields(logrus.Fields{
		"repo":    "cbt-api",
		"network": network.Name,
	}).Info("generating protos")

	// Use the generated config file
	configPath := filepath.Join(".xcli", "configs", fmt.Sprintf("cbt-api-%s.yaml", network.Name))

	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute config path: %w", err)
	}

	cmd := exec.CommandContext(ctx, "make", "proto")
	cmd.Dir = m.cfg.Repos.CBTAPI

	cmd.Env = append(os.Environ(), fmt.Sprintf("CONFIG_FILE=%s", absConfigPath))

	if err := executil.RunCmd(cmd, m.verbose); err != nil {
		return fmt.Errorf("failed to generate cbt-api protos: %w", err)
	}

	return nil
}

// binaryExists checks if a binary file exists.
func (m *Manager) binaryExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir() && info.Mode()&0111 != 0 // Check if executable
}

// dirExists checks if a directory exists.
func (m *Manager) dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.IsDir()
}

// runMake runs make with a target in a directory.
func (m *Manager) runMake(ctx context.Context, dir string, target string) error {
	cmd := exec.CommandContext(ctx, "make", target)
	cmd.Dir = dir

	if err := executil.RunCmd(cmd, m.verbose); err != nil {
		return fmt.Errorf("make %s failed: %w", target, err)
	}

	return nil
}

// CheckBinariesExist checks if all required binaries exist.
func (m *Manager) CheckBinariesExist() map[string]bool {
	return map[string]bool{
		"xatu-cbt":    m.binaryExists(filepath.Join(m.cfg.Repos.XatuCBT, "bin", "xatu-cbt")),
		"cbt":         m.binaryExists(filepath.Join(m.cfg.Repos.CBT, "bin", "cbt")),
		"cbt-api":     m.binaryExists(filepath.Join(m.cfg.Repos.CBTAPI, "bin", "server")),
		"lab-backend": m.binaryExists(filepath.Join(m.cfg.Repos.LabBackend, "bin", "lab-backend")),
		"lab-deps":    m.dirExists(filepath.Join(m.cfg.Repos.Lab, "node_modules")),
	}
}
