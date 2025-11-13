package infrastructure

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/sirupsen/logrus"
)

// Manager handles infrastructure via xatu-cbt.
type Manager struct {
	log         logrus.FieldLogger
	cfg         *config.LabConfig
	xatuCBTPath string
	verbose     bool
}

// NewManager creates a new infrastructure manager.
func NewManager(log logrus.FieldLogger, cfg *config.LabConfig) *Manager {
	xatuCBTPath := cfg.Repos.XatuCBT + "/bin/xatu-cbt"

	return &Manager{
		log:         log.WithField("component", "infrastructure"),
		cfg:         cfg,
		xatuCBTPath: xatuCBTPath,
		verbose:     false,
	}
}

// SetVerbose sets verbose mode for infrastructure commands.
func (m *Manager) SetVerbose(verbose bool) {
	m.verbose = verbose
}

// runCmd runs a command with appropriate output handling.
func (m *Manager) runCmd(cmd *exec.Cmd) error {
	if m.verbose {
		// Verbose mode: show all output in real-time
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		return cmd.Run()
	}

	// Quiet mode: capture output, only show if command fails
	var output bytes.Buffer

	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		// Command failed - show captured output
		if output.Len() > 0 {
			os.Stderr.Write(output.Bytes())
		}

		return err
	}

	// Command succeeded - no output
	return nil
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

	// Run xatu-cbt infra start with xatu-source flag
	xatuSource := constants.InfraModeLocal
	if m.cfg.Infrastructure.ClickHouse.Xatu.Mode == constants.InfraModeExternal {
		xatuSource = constants.InfraModeExternal
	}

	m.log.WithField("xatu_source", xatuSource).Info("starting infrastructure")

	// Build command arguments
	args := []string{"infra", "start", "--xatu-source", xatuSource}

	// Add external Xatu URL if in external mode
	if xatuSource == constants.InfraModeExternal {
		if m.cfg.Infrastructure.ClickHouse.Xatu.ExternalURL == "" {
			return fmt.Errorf("external URL is required when using external Xatu source")
		}

		args = append(args, "--xatu-url", m.cfg.Infrastructure.ClickHouse.Xatu.ExternalURL)
	}

	//nolint:gosec // xatuCBTPath is from config and validated during discovery
	cmd := exec.CommandContext(ctx, m.xatuCBTPath, args...)
	cmd.Dir = m.cfg.Repos.XatuCBT

	if err := m.runCmd(cmd); err != nil {
		return fmt.Errorf("failed to start infrastructure: %w", err)
	}

	// Wait for services to be ready
	if err := m.WaitForReady(ctx, 120*time.Second); err != nil {
		return fmt.Errorf("infrastructure did not become ready: %w", err)
	}

	m.log.Info("infrastructure started successfully")

	return nil
}

// Stop stops infrastructure via xatu-cbt.
func (m *Manager) Stop(ctx context.Context) error {
	m.log.Info("stopping infrastructure")

	//nolint:gosec // xatuCBTPath is from config and validated during discovery
	cmd := exec.CommandContext(ctx, m.xatuCBTPath, "infra", "stop")
	cmd.Dir = m.cfg.Repos.XatuCBT

	if err := m.runCmd(cmd); err != nil {
		return fmt.Errorf("failed to stop infrastructure: %w", err)
	}

	m.log.Info("infrastructure stopped")

	return nil
}

// Reset resets infrastructure (clean slate).
func (m *Manager) Reset(ctx context.Context) error {
	m.log.Info("resetting infrastructure")

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

	m.log.Info("infrastructure reset complete")

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
	// Check if ClickHouse CBT is accessible
	if !m.checkPort("localhost:8123") {
		return false
	}

	// Check if Redis is accessible
	if !m.checkPort("localhost:6380") {
		return false
	}

	return true
}

// WaitForReady waits for infrastructure to be ready.
func (m *Manager) WaitForReady(ctx context.Context, timeout time.Duration) error {
	m.log.Info("waiting for infrastructure to be ready")

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	checks := []struct {
		name string
		addr string
	}{
		{"ClickHouse CBT", "localhost:8123"},
		{"Redis", "localhost:6380"},
	}

	// Add Xatu ClickHouse check if in local mode
	if m.cfg.Infrastructure.ClickHouse.Xatu.Mode == "local" {
		checks = append(checks, struct {
			name string
			addr string
		}{"ClickHouse Xatu", "localhost:8125"})
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for infrastructure")
		case <-ticker.C:
			allReady := true

			for _, check := range checks {
				if !m.checkPort(check.addr) {
					m.log.WithField("service", check.name).Debug("waiting")

					allReady = false

					break
				}
			}

			if allReady {
				m.log.Info("all infrastructure services are ready")

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
	status := map[string]bool{
		"ClickHouse CBT":  m.checkPort("localhost:8123"),
		"ClickHouse Xatu": m.checkPort("localhost:8125"),
		"Redis":           m.checkPort("localhost:6380"),
	}

	return status
}
