// Package infrastructure manages Docker-based infrastructure services (ClickHouse, Redis)
// via xatu-cbt, including health checks, migrations, and mode-specific configuration.
package infrastructure

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2" // clickhouse database driver
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	executil "github.com/ethpandaops/xcli/pkg/exec"
	"github.com/ethpandaops/xcli/pkg/mode"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/clickhouse" // migrate clickhouse driver
	_ "github.com/golang-migrate/migrate/v4/source/file"         // migrate file source driver
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

const (
	// infrastructureReadyTimeout is the maximum time to wait for infrastructure to become ready.
	infrastructureReadyTimeout = 120 * time.Second
)

// Manager handles infrastructure via xatu-cbt.
type Manager struct {
	log         logrus.FieldLogger
	cfg         *config.LabConfig
	mode        mode.Mode
	xatuCBTPath string
	verbose     bool
}

// NewManager creates a new infrastructure manager.
// Mode parameter provides mode-specific behavior (services, ports, etc.)
func NewManager(log logrus.FieldLogger, cfg *config.LabConfig, m mode.Mode) *Manager {
	xatuCBTPath := cfg.Repos.XatuCBT + "/bin/xatu-cbt"

	return &Manager{
		log:         log.WithField("component", "infrastructure"),
		cfg:         cfg,
		mode:        m,
		xatuCBTPath: xatuCBTPath,
		verbose:     false,
	}
}

// SetVerbose sets verbose mode for infrastructure commands.
func (m *Manager) SetVerbose(verbose bool) {
	m.verbose = verbose
}

// runCmd runs a command using the shared exec runner.
func (m *Manager) runCmd(cmd *exec.Cmd) error {
	return executil.RunCmd(cmd, m.verbose)
}

// Start starts infrastructure via xatu-cbt.
func (m *Manager) Start(ctx context.Context) error {
	// Check if infrastructure is already running
	if m.IsRunning() {
		m.log.Info("infrastructure is already running")

		return nil
	}

	// Check if xatu-cbt binary exists
	if _, err := os.Stat(m.xatuCBTPath); os.IsNotExist(err) {
		return fmt.Errorf("xatu-cbt binary not found at %s - please run 'make build' in xatu-cbt", m.xatuCBTPath)
	}

	// Determine xatu-source from mode (instead of hard-coded config check)
	xatuSource := constants.InfraModeLocal
	if m.mode.NeedsExternalClickHouse() {
		xatuSource = constants.InfraModeExternal
	}

	m.log.WithFields(logrus.Fields{
		"mode":        m.mode.Name(),
		"xatu_source": xatuSource,
	}).Info("starting infrastructure")

	// Create spinner for infrastructure startup
	spinner := ui.NewSpinner("Starting infrastructure services")

	// Build command arguments
	args := []string{"infra", "start", "--xatu-source", xatuSource}

	// Add external Xatu URL if in external mode
	if xatuSource == constants.InfraModeExternal {
		args = append(args, "--xatu-url", m.cfg.Infrastructure.ClickHouse.Xatu.ExternalURL)
	}

	//nolint:gosec // xatuCBTPath is from config and validated during discovery
	cmd := exec.CommandContext(ctx, m.xatuCBTPath, args...)
	cmd.Dir = m.cfg.Repos.XatuCBT

	if err := m.runCmd(cmd); err != nil {
		spinner.Fail("Failed to start infrastructure services")

		return fmt.Errorf("failed to start infrastructure: %w", err)
	}

	// Wait for services to be ready
	spinner.UpdateText("Waiting for services to be healthy")

	if err := m.WaitForReady(ctx, infrastructureReadyTimeout, spinner); err != nil {
		spinner.Fail("Infrastructure failed to become ready")

		return fmt.Errorf("infrastructure did not become ready: %w", err)
	}

	spinner.Success("Infrastructure services are healthy")

	// Run xatu migrations if in local mode (external Xatu already has schema)
	if xatuSource == constants.InfraModeLocal {
		m.log.Info("running xatu migrations against local cluster")

		if err := m.runXatuMigrations(ctx); err != nil {
			return fmt.Errorf("failed to run xatu migrations: %w", err)
		}

		m.log.Info("xatu migrations completed successfully")
	}

	return nil
}

// Stop stops infrastructure via xatu-cbt.
func (m *Manager) Stop(ctx context.Context) error {
	m.log.WithField("mode", m.mode.Name()).Info("stopping infrastructure")

	//nolint:gosec // xatuCBTPath is from config and validated during discovery
	cmd := exec.CommandContext(ctx, m.xatuCBTPath, "infra", "stop")
	cmd.Dir = m.cfg.Repos.XatuCBT

	if err := m.runCmd(cmd); err != nil {
		return fmt.Errorf("failed to stop infrastructure: %w", err)
	}

	m.log.WithField("mode", m.mode.Name()).Info("infrastructure stopped")

	return nil
}

// Reset resets infrastructure (clean slate).
func (m *Manager) Reset(ctx context.Context) error {
	m.log.WithField("mode", m.mode.Name()).Info("resetting infrastructure")

	// Stop first
	if err := m.Stop(ctx); err != nil {
		m.log.WithError(err).Warn("Failed to stop infrastructure")
	}

	// Remove volumes
	//nolint:gosec // xatuCBTPath is from config and validated during discovery
	cmd := exec.CommandContext(ctx, m.xatuCBTPath, "infra", "reset")
	cmd.Dir = m.cfg.Repos.XatuCBT

	if err := m.runCmd(cmd); err != nil {
		return fmt.Errorf("failed to reset infrastructure: %w", err)
	}

	m.log.WithField("mode", m.mode.Name()).Info("infrastructure reset complete")

	return nil
}

// SetupNetwork runs migrations for a network.
func (m *Manager) SetupNetwork(ctx context.Context, network string) error {
	m.log.WithField("network", network).Info("running network setup")

	// xatu-cbt network setup uses NETWORK env var, not --network flag
	//nolint:gosec // xatuCBTPath is from config and validated during discovery
	cmd := exec.CommandContext(ctx, m.xatuCBTPath, "network", "setup", "--force")
	cmd.Dir = m.cfg.Repos.XatuCBT

	cmd.Env = append(os.Environ(), fmt.Sprintf("NETWORK=%s", network))

	if err := m.runCmd(cmd); err != nil {
		return fmt.Errorf("failed to setup network %s: %w", network, err)
	}

	return nil
}

// IsRunning checks if infrastructure is running.
func (m *Manager) IsRunning() bool {
	// Get ports from mode (instead of hard-coded ports)
	ports := m.mode.GetHealthCheckPorts()

	for _, port := range ports {
		addr := fmt.Sprintf("localhost:%d", port)
		if !m.checkPort(addr) {
			return false
		}
	}

	return true
}

// WaitForReady waits for infrastructure to be ready.
func (m *Manager) WaitForReady(ctx context.Context, timeout time.Duration, spinner *ui.Spinner) error {
	// Get ports from mode (instead of hard-coded checks)
	ports := m.mode.GetHealthCheckPorts()

	m.log.WithFields(logrus.Fields{
		"mode":  m.mode.Name(),
		"ports": len(ports),
	}).Info("waiting for infrastructure to be ready")

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	attempt := 0
	maxAttempts := int(timeout / (2 * time.Second))

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for infrastructure (%s mode)", m.mode.Name())
		case <-ticker.C:
			attempt++
			spinner.UpdateText(fmt.Sprintf("Health check %d/%d - waiting for services", attempt, maxAttempts))

			allReady := true

			for _, port := range ports {
				addr := fmt.Sprintf("localhost:%d", port)
				if !m.checkPort(addr) {
					m.log.WithField("port", port).Debug("waiting for port")

					allReady = false

					break
				}
			}

			if allReady {
				m.log.WithField("mode", m.mode.Name()).Info("all infrastructure services are ready")

				return nil
			}
		}
	}
}

// checkPort checks if a port is open.
func (m *Manager) checkPort(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		return false
	}

	conn.Close()

	return true
}

// Status returns the status of infrastructure components.
func (m *Manager) Status() map[string]bool {
	// Get ports from mode (instead of hard-coded ports)
	ports := m.mode.GetHealthCheckPorts()

	status := make(map[string]bool, len(ports))

	// Map port numbers to service names based on configuration
	portNames := map[int]string{
		m.cfg.Infrastructure.ClickHouseCBTPort:  "ClickHouse CBT",
		m.cfg.Infrastructure.ClickHouseXatuPort: "ClickHouse Xatu",
		m.cfg.Infrastructure.RedisPort:          "Redis",
	}

	for _, port := range ports {
		addr := fmt.Sprintf("localhost:%d", port)

		name := portNames[port]

		if name == "" {
			name = fmt.Sprintf("Port %d", port)
		}

		status[name] = m.checkPort(addr)
	}

	return status
}

// TestExternalConnection tests connectivity to external ClickHouse using docker.
func (m *Manager) TestExternalConnection(ctx context.Context) error {
	// Parse the external URL to extract host, port, and credentials
	externalURL := m.cfg.Infrastructure.ClickHouse.Xatu.ExternalURL

	parsedURL, err := url.Parse(externalURL)
	if err != nil {
		return fmt.Errorf("failed to parse external URL: %w", err)
	}

	// Extract host and port
	host := parsedURL.Hostname()

	port := parsedURL.Port()
	if port == "" {
		// Default ClickHouse native port
		port = "9000"
	}

	// Extract username and password
	username := "default"
	password := ""

	if parsedURL.User != nil {
		if parsedURL.User.Username() != "" {
			username = parsedURL.User.Username()
		}

		if pass, ok := parsedURL.User.Password(); ok {
			password = pass
		}
	}

	// Use configured credentials if available
	if m.cfg.Infrastructure.ClickHouse.Xatu.ExternalUsername != "" {
		username = m.cfg.Infrastructure.ClickHouse.Xatu.ExternalUsername
	}

	if m.cfg.Infrastructure.ClickHouse.Xatu.ExternalPassword != "" {
		password = m.cfg.Infrastructure.ClickHouse.Xatu.ExternalPassword
	}

	// Build docker command to test connection
	args := []string{
		"run", "--rm",
		"clickhouse/clickhouse-client:latest",
		"--host", host,
		"--port", port,
		"--user", username,
	}

	if password != "" {
		args = append(args, "--password", password)
	}

	args = append(args, "--query", "SELECT 1")

	m.log.WithFields(logrus.Fields{
		"host": host,
		"port": port,
		"user": username,
	}).Debug("testing external ClickHouse connection")

	cmd := exec.CommandContext(ctx, "docker", args...)

	var output bytes.Buffer

	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(output.String())
		if errMsg == "" {
			errMsg = err.Error()
		}

		return fmt.Errorf("connection test failed: %s", errMsg)
	}

	// Verify output contains "1" (may have Docker warnings or other messages)
	result := strings.TrimSpace(output.String())

	// Check if the last non-empty line is "1"
	lines := strings.Split(result, "\n")
	lastLine := ""

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			lastLine = line

			break
		}
	}

	if lastLine != "1" {
		return fmt.Errorf("unexpected response from ClickHouse (expected '1', got '%s'): %s", lastLine, result)
	}

	return nil
}

// parseXatuCBTEnv parses the xatu-cbt .env file and returns key-value pairs.
func (m *Manager) parseXatuCBTEnv() (map[string]string, error) {
	envPath := filepath.Join(m.cfg.Repos.XatuCBT, ".env")

	env, err := godotenv.Read(envPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read xatu-cbt .env file: %w", err)
	}

	return env, nil
}

// runXatuMigrations runs xatu schema migrations against the local xatu-clickhouse cluster.
// This creates the external source tables (beacon_api_*, canonical_*, mev_relay_*, etc.)
// that CBT transformations depend on.
func (m *Manager) runXatuMigrations(ctx context.Context) error {
	spinner := ui.NewSilentSpinner("Running database migrations")

	// Path to xatu migrations (cloned by xatu-cbt infra start)
	migrationsPath := filepath.Join(m.cfg.Repos.XatuCBT, "xatu", "deploy", "migrations", "clickhouse")

	// Check if migrations directory exists
	if _, err := os.Stat(migrationsPath); os.IsNotExist(err) {
		spinner.Fail("Database migrations failed")

		return fmt.Errorf("xatu migrations not found at %s - xatu repo may not be cloned", migrationsPath)
	}

	// Parse xatu-cbt .env to get ClickHouse credentials
	env, err := m.parseXatuCBTEnv()
	if err != nil {
		spinner.Fail("Database migrations failed")

		return fmt.Errorf("failed to parse xatu-cbt .env: %w", err)
	}

	// Extract connection parameters from .env
	host := env["CLICKHOUSE_HOST"]
	if host == "" {
		host = "localhost" // fallback default
	}

	port := env["CLICKHOUSE_XATU_01_NATIVE_PORT"]
	if port == "" {
		port = "9002" // fallback default
	}

	username := env["CLICKHOUSE_USERNAME"]
	if username == "" {
		username = "default" // fallback default
	}

	password := env["CLICKHOUSE_PASSWORD"]
	if password == "" {
		password = "" // no fallback for security
	}

	// Build connection string for xatu-clickhouse cluster
	// Using native port (from CLICKHOUSE_XATU_01_NATIVE_PORT) for golang-migrate (more reliable than HTTP)
	hostPort := net.JoinHostPort(host, port)
	connStr := fmt.Sprintf(
		"clickhouse://%s?username=%s&password=%s&database=default&x-multi-statement=true",
		hostPort, username, password,
	)

	m.log.WithField("migrations_path", migrationsPath).Debug("initializing xatu migrations")

	// Create migration instance
	migration, err := migrate.New(
		fmt.Sprintf("file://%s", migrationsPath),
		connStr,
	)
	if err != nil {
		spinner.Fail("Database migrations failed")

		return fmt.Errorf("failed to create migration instance: %w", err)
	}

	defer func() {
		if _, closeErr := migration.Close(); closeErr != nil {
			m.log.WithError(closeErr).Warn("failed to close migration instance")
		}
	}()

	// Run migrations
	spinner.UpdateText("Applying database migrations")

	err = migration.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		spinner.Fail("Database migrations failed")

		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		m.log.Debug("no new xatu migrations to apply")
		// Stop silently - parent Start() spinner shows overall success
		_ = spinner.Stop()
	} else {
		// Get version to log what was applied
		version, dirty, vErr := migration.Version()
		if vErr != nil && !errors.Is(vErr, migrate.ErrNilVersion) {
			spinner.Fail("Database migrations failed")

			return fmt.Errorf("failed to get migration version: %w", vErr)
		}

		if dirty {
			spinner.Fail("Database migrations failed")

			return fmt.Errorf("migration left database in dirty state")
		}

		m.log.WithField("version", version).Info("xatu migrations applied")
		// Stop silently - parent Start() spinner shows overall success
		_ = spinner.Stop()
	}

	return nil
}
