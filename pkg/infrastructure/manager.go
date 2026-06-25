// Package infrastructure manages Docker-based infrastructure services (ClickHouse, Redis)
// via xatu-cbt, including health checks, migrations, and mode-specific configuration.
package infrastructure

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/configgen"
	"github.com/ethpandaops/xcli/pkg/constants"
	executil "github.com/ethpandaops/xcli/pkg/exec"
	"github.com/ethpandaops/xcli/pkg/instance"
	"github.com/ethpandaops/xcli/pkg/mode"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
)

const (
	// infrastructureReadyTimeout is the maximum time to wait for infrastructure to become ready.
	infrastructureReadyTimeout = 120 * time.Second

	// clickHouseClusterHostPrefix is the hostname prefix for ClickHouse clusters.
	// When this prefix is detected, additional DNS checks are performed for individual shards.
	clickHouseClusterHostPrefix = "chendpoint-clickhouse-raw"

	// logFieldHost is the structured log field key for a host.
	logFieldHost = "host"

	defaultXatuCBTProjectName = "xatu-cbt-platform"

	// cmdInfra is the xatu-cbt "infra" subcommand.
	cmdInfra = "infra"
	// flagProjectName is the xatu-cbt "--project-name" flag.
	flagProjectName = "--project-name"
)

// clickHouseClusterShards are the shard suffixes for ClickHouse clusters.
var clickHouseClusterShards = []string{"0-0", "0-1", "1-0", "1-1", "2-0", "2-1"}

// Manager handles infrastructure via xatu-cbt.
type Manager struct {
	log           logrus.FieldLogger
	cfg           *config.LabConfig
	mode          mode.Mode
	xatuCBTPath   string
	verbose       bool
	observability *ObservabilityManager
	xcliDir       string
	runtime       *instance.Runtime
	runCmd        func(*exec.Cmd, bool) error
}

// NewManager creates a new infrastructure manager.
// Mode parameter provides mode-specific behavior (services, ports, etc.)
func NewManager(log logrus.FieldLogger, cfg *config.LabConfig, m mode.Mode, xcliDir string) *Manager {
	return NewManagerWithRuntime(log, cfg, m, xcliDir, nil)
}

// NewManagerWithRuntime creates an infrastructure manager bound to an instance runtime.
func NewManagerWithRuntime(
	log logrus.FieldLogger,
	cfg *config.LabConfig,
	m mode.Mode,
	xcliDir string,
	runtime *instance.Runtime,
) *Manager {
	xatuCBTPath := cfg.Repos.XatuCBT + "/bin/xatu-cbt"

	if runtime != nil && runtime.Manifest != nil && runtime.Manifest.StateDir != "" {
		xcliDir = runtime.Manifest.StateDir
	}

	return &Manager{
		log:         log.WithField("component", "infrastructure"),
		cfg:         cfg,
		mode:        m,
		xatuCBTPath: xatuCBTPath,
		verbose:     false,
		xcliDir:     xcliDir,
		runtime:     runtime,
		runCmd:      executil.RunCmd,
	}
}

// SetVerbose sets verbose mode for infrastructure commands.
func (m *Manager) SetVerbose(verbose bool) {
	m.verbose = verbose
}

// DockerContainerName returns the Docker container name for xcli-managed services.
func (m *Manager) DockerContainerName(service string) (string, bool) {
	if service != constants.ServicePrometheus && service != constants.ServiceGrafana {
		return "", false
	}

	resources := newObservabilityResources(m.cfg, m.runtime)

	name := resources.containers[service]
	if name == "" {
		return "", false
	}

	return name, true
}

func (m *Manager) userStateDir() string {
	if m.runtime != nil && m.runtime.Workspace != nil && m.runtime.Workspace.StateDir != "" {
		return m.runtime.Workspace.StateDir
	}

	return m.xcliDir
}

func (m *Manager) xatuCBTProjectName() string {
	if m.runtime == nil {
		return defaultXatuCBTProjectName
	}

	if plan := m.runtime.EffectiveDockerPlan(); plan.ProjectName != "" {
		return plan.ProjectName
	}

	return defaultXatuCBTProjectName
}

func (m *Manager) infraStartArgs(xatuSource string) []string {
	return []string{
		cmdInfra,
		"start",
		flagProjectName,
		m.xatuCBTProjectName(),
		"--xatu-source",
		xatuSource,
	}
}

func (m *Manager) infraActionArgs(action string) []string {
	return []string{
		cmdInfra,
		action,
		flagProjectName,
		m.xatuCBTProjectName(),
	}
}

func (m *Manager) xatuCBTCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	//nolint:gosec // G204: args are internally constructed, not user input
	cmd := exec.CommandContext(ctx, m.xatuCBTPath, args...)
	cmd.Dir = m.cfg.Repos.XatuCBT

	env, err := m.xatuCBTEnv(os.Environ())
	if err != nil {
		return nil, err
	}

	cmd.Env = env

	return cmd, nil
}

func (m *Manager) runXatuCBT(ctx context.Context, args ...string) error {
	cmd, err := m.xatuCBTCommand(ctx, args...)
	if err != nil {
		return err
	}

	runCmd := m.runCmd
	if runCmd == nil {
		runCmd = executil.RunCmd
	}

	return runCmd(cmd, m.verbose)
}

func (m *Manager) xatuCBTEnv(base []string) ([]string, error) {
	plan, err := m.xatuCBTPortPlan()
	if err != nil {
		return nil, err
	}

	return mergeEnv(base, plan.XatuCBTEnv(m.xatuCBTProjectName())), nil
}

func (m *Manager) xatuCBTPortPlan() (instance.PortPlan, error) {
	fallback, err := instance.BuildPortPlan(m.cfg, 0)
	if err != nil {
		return instance.PortPlan{}, err
	}

	if m.runtime == nil {
		return fallback, nil
	}

	plan := m.runtime.Ports
	if len(plan.AllPorts()) == 0 && m.runtime.Manifest != nil {
		plan = m.runtime.Manifest.Ports
	}

	return plan.WithDefaults(fallback), nil
}

func (m *Manager) healthCheckPorts() ([]int, error) {
	if m.runtime == nil {
		return m.mode.GetHealthCheckPorts(), nil
	}

	plan, err := m.xatuCBTPortPlan()
	if err != nil {
		return nil, err
	}

	ports := []int{plan.ClickHouseCBT01HTTP, plan.Redis}
	if !m.mode.NeedsExternalClickHouse() {
		ports = append(ports, plan.ClickHouseXatu01HTTP)
	}

	return ports, nil
}

func mergeEnv(base []string, overrides map[string]string) []string {
	env := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, found := strings.Cut(entry, "=")
		if found && overrides[name] != "" {
			continue
		}

		env = append(env, entry)
	}

	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		env = append(env, fmt.Sprintf("%s=%s", name, overrides[name]))
	}

	return env
}

// Start starts infrastructure via xatu-cbt.
func (m *Manager) Start(ctx context.Context) error {
	// Check if infrastructure is already running
	if m.IsRunning(ctx) {
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
	args := m.infraStartArgs(xatuSource)

	// Add external Xatu URL if in external mode. Credentials configured via the
	// separate externalUsername/externalPassword fields must be embedded into the
	// URL, since xatu-cbt reads them only from the URL when generating its cluster
	// config.
	//
	// Trade-off: embedding credentials means the password appears in this child
	// process's command line (world-readable via /proc/<pid>/cmdline on Linux)
	// for the lifetime of the short-lived `infra start` call. xatu-cbt's
	// infra-start currently has no env/file alternative to the --xatu-url flag;
	// switching to an env var (as the builder path uses) would require a change
	// there. The password is also already persisted to disk in xatu-cbt's
	// generated ClickHouse config, so this does not introduce on-disk exposure.
	if xatuSource == constants.InfraModeExternal {
		xatuURL, urlErr := m.cfg.Infrastructure.ClickHouse.Xatu.ExternalURLWithCredentials()
		if urlErr != nil {
			spinner.Fail("Failed to start infrastructure services")

			return fmt.Errorf("failed to build external Xatu URL: %w", urlErr)
		}

		args = append(args, "--xatu-url", xatuURL)
	}

	// Add xatu ref if configured
	if m.cfg.Dev.XatuRef != "" {
		args = append(args, "--xatu-ref", m.cfg.Dev.XatuRef)
	}

	if err := m.runXatuCBT(ctx, args...); err != nil {
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

	// Run xatu migrations if in local mode (external Xatu already has schema).
	// Delegated to the xatu-cbt binary, which owns the xatu migration logic.
	if xatuSource == constants.InfraModeLocal {
		m.log.Info("running xatu migrations against local cluster")

		if err := m.migrateXatu(ctx); err != nil {
			return fmt.Errorf("failed to run xatu migrations: %w", err)
		}

		m.log.Info("xatu migrations completed successfully")
	}

	// Start observability stack if enabled
	if m.cfg.Infrastructure.Observability.Enabled {
		if err := m.startObservability(ctx); err != nil {
			return fmt.Errorf("failed to start observability stack: %w", err)
		}
	}

	return nil
}

// Stop stops infrastructure via xatu-cbt.
func (m *Manager) Stop(ctx context.Context) error {
	m.log.WithField("mode", m.mode.Name()).Info("stopping infrastructure")

	// Stop observability stack first if enabled
	if m.cfg.Infrastructure.Observability.Enabled {
		if err := m.stopObservability(ctx); err != nil {
			m.log.WithError(err).Warn("failed to stop observability stack")
		}
	}

	if err := m.runXatuCBT(ctx, m.infraActionArgs("stop")...); err != nil {
		return fmt.Errorf("failed to stop infrastructure: %w", err)
	}

	m.log.WithField("mode", m.mode.Name()).Info("infrastructure stopped")

	return nil
}

// Reset resets infrastructure and removes data. Use Stop for ordinary shutdown.
func (m *Manager) Reset(ctx context.Context) error {
	return m.Destroy(ctx)
}

// Destroy removes infrastructure containers and volumes for this instance.
func (m *Manager) Destroy(ctx context.Context) error {
	m.log.WithField("mode", m.mode.Name()).Info("resetting infrastructure")

	if m.cfg.Infrastructure.Observability.Enabled {
		if m.observability == nil {
			obsMgr, err := NewObservabilityManagerWithRuntime(m.log, m.cfg, m.xcliDir, m.runtime)
			if err != nil {
				return fmt.Errorf("failed to create observability manager: %w", err)
			}

			m.observability = obsMgr
		}

		if err := m.observability.Destroy(ctx); err != nil {
			m.log.WithError(err).Warn("failed to destroy observability stack")
		}
	}

	if err := m.runXatuCBT(ctx, m.infraActionArgs("reset")...); err != nil {
		return fmt.Errorf("failed to reset infrastructure: %w", err)
	}

	m.log.WithField("mode", m.mode.Name()).Info("infrastructure reset complete")

	return nil
}

// ResetRedis intentionally clears Redis data for this instance only.
func (m *Manager) ResetRedis(ctx context.Context) error {
	plan, err := m.xatuCBTPortPlan()
	if err != nil {
		return err
	}

	//nolint:gosec // G204: args are internally constructed, not user input
	cmd := exec.CommandContext(
		ctx,
		"redis-cli",
		"-h",
		"127.0.0.1",
		"-p",
		strconv.Itoa(plan.Redis),
		"FLUSHALL",
	)

	runCmd := m.runCmd
	if runCmd == nil {
		runCmd = executil.RunCmd
	}

	if err := runCmd(cmd, m.verbose); err != nil {
		return fmt.Errorf("failed to reset Redis for instance %s: %w", m.xatuCBTProjectName(), err)
	}

	return nil
}

// SetupNetwork runs migrations for a network.
func (m *Manager) SetupNetwork(ctx context.Context, network string) error {
	m.log.WithField("network", network).Info("running network setup")

	// xatu-cbt network setup uses NETWORK env var, not --network flag
	cmd, err := m.xatuCBTCommand(ctx, "network", "setup", "--force")
	if err != nil {
		return err
	}

	cmd.Env = mergeEnv(cmd.Env, map[string]string{"NETWORK": network})

	runCmd := m.runCmd
	if runCmd == nil {
		runCmd = executil.RunCmd
	}

	if err := runCmd(cmd, m.verbose); err != nil {
		return fmt.Errorf("failed to setup network %s: %w", network, err)
	}

	return nil
}

// IsRunning checks if infrastructure is running.
func (m *Manager) IsRunning(ctx context.Context) bool {
	ports, err := m.healthCheckPorts()
	if err != nil {
		m.log.WithError(err).Warn("failed to resolve infrastructure health check ports")

		return false
	}

	for _, port := range ports {
		addr := fmt.Sprintf("localhost:%d", port)
		if !m.checkPort(ctx, addr) {
			return false
		}
	}

	return true
}

// WaitForReady waits for infrastructure to be ready.
func (m *Manager) WaitForReady(ctx context.Context, timeout time.Duration, spinner *ui.Spinner) error {
	ports, err := m.healthCheckPorts()
	if err != nil {
		return fmt.Errorf("failed to resolve infrastructure health check ports: %w", err)
	}

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
				if !m.checkPort(ctx, addr) {
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

// Status returns the status of infrastructure components relevant to the current mode.
// Hybrid mode shows ClickHouse CBT + Redis; local mode adds ClickHouse Xatu.
func (m *Manager) Status(ctx context.Context) map[string]bool {
	// Map ports to display names for mode-relevant infrastructure only.
	plan, err := m.xatuCBTPortPlan()
	if err != nil {
		m.log.WithError(err).Warn("failed to resolve infrastructure status ports")

		return map[string]bool{}
	}

	portNames := map[int]string{
		plan.ClickHouseCBT01HTTP: "ClickHouse CBT",
		plan.Redis:               "Redis",
	}

	// Local mode also runs a local ClickHouse Xatu instance.
	if !m.mode.NeedsExternalClickHouse() {
		portNames[plan.ClickHouseXatu01HTTP] = "ClickHouse Xatu"
	}

	status := make(map[string]bool, len(portNames))

	for port, name := range portNames {
		addr := fmt.Sprintf("localhost:%d", port)
		status[name] = m.checkPort(ctx, addr)
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

	// Check DNS for cluster shards if using a known ClickHouse cluster endpoint
	if err := m.checkClusterShardDNS(ctx, host); err != nil {
		return err
	}

	port := parsedURL.Port()
	if port == "" {
		// Default ClickHouse native port
		port = strconv.Itoa(constants.DefaultClickHouseCBTNativePort)
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
		logFieldHost: host,
		"port":       port,
		"user":       username,
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

// GetObservabilityStatus returns the status of observability containers.
// Returns an empty map if observability is disabled.
func (m *Manager) GetObservabilityStatus(ctx context.Context) (map[string]ContainerStatus, error) {
	if !m.cfg.Infrastructure.Observability.Enabled {
		return make(map[string]ContainerStatus), nil
	}

	if m.observability == nil {
		obsMgr, err := NewObservabilityManagerWithRuntime(m.log, m.cfg, m.xcliDir, m.runtime)
		if err != nil {
			return nil, fmt.Errorf("failed to create observability manager: %w", err)
		}

		m.observability = obsMgr
	}

	return m.observability.Status(ctx)
}

// RestartObservabilityService restarts a specific observability service.
func (m *Manager) RestartObservabilityService(ctx context.Context, service string) error {
	if !m.cfg.Infrastructure.Observability.Enabled {
		return fmt.Errorf("observability is not enabled")
	}

	if m.observability == nil {
		obsMgr, err := NewObservabilityManagerWithRuntime(m.log, m.cfg, m.xcliDir, m.runtime)
		if err != nil {
			return fmt.Errorf("failed to create observability manager: %w", err)
		}

		m.observability = obsMgr
	}

	return m.observability.RestartService(ctx, service)
}

// StartObservabilityService starts a specific observability service container.
func (m *Manager) StartObservabilityService(ctx context.Context, service string) error {
	if !m.cfg.Infrastructure.Observability.Enabled {
		return fmt.Errorf("observability is not enabled")
	}

	if m.observability == nil {
		obsMgr, err := NewObservabilityManagerWithRuntime(m.log, m.cfg, m.xcliDir, m.runtime)
		if err != nil {
			return fmt.Errorf("failed to create observability manager: %w", err)
		}

		m.observability = obsMgr
	}

	return m.observability.StartService(ctx, service)
}

// StopObservabilityService stops a specific observability service container.
func (m *Manager) StopObservabilityService(ctx context.Context, service string) error {
	if !m.cfg.Infrastructure.Observability.Enabled {
		return fmt.Errorf("observability is not enabled")
	}

	if m.observability == nil {
		obsMgr, err := NewObservabilityManagerWithRuntime(m.log, m.cfg, m.xcliDir, m.runtime)
		if err != nil {
			return fmt.Errorf("failed to create observability manager: %w", err)
		}

		m.observability = obsMgr
	}

	return m.observability.StopService(ctx, service)
}

// AutoSeedBoundsIfNeeded checks if local Redis has external model bounds and seeds them
// from production if missing. External bounds tell CBT the min/max range of data available
// on the external ClickHouse, avoiding slow initial full scans.
func (m *Manager) AutoSeedBoundsIfNeeded(ctx context.Context, spinner ui.Task) error {
	// Only seed in hybrid mode (external xatu + local xatu-cbt)
	if !m.mode.NeedsExternalClickHouse() {
		return nil
	}

	m.log.Debug("Checking if external bounds seeding is needed...")

	seeder := NewBoundsSeederWithRuntime(m.log, m.runtime)

	enabledNetworks := m.cfg.EnabledNetworks()
	if len(enabledNetworks) == 0 {
		m.log.Warn("No networks enabled, skipping bounds seeding")

		return nil
	}

	seededCount := 0
	skippedCount := 0

	for i, network := range enabledNetworks {
		redisDB := i // mainnet=0, sepolia=1, hoodi=2, etc.

		spinner.UpdateText(fmt.Sprintf("Checking external bounds for %s", network.Name))

		needsSeeding, err := seeder.CheckNeedsSeeding(ctx, redisDB)
		if err != nil {
			m.log.WithError(err).WithField("network", network.Name).
				Warn("Failed to check external bounds status (non-fatal)")

			continue
		}

		if !needsSeeding {
			m.log.WithField("network", network.Name).Debug("External bounds already seeded, skipping")

			skippedCount++

			continue
		}

		spinner.UpdateText(fmt.Sprintf("Seeding external bounds for %s", network.Name))

		m.log.WithField("network", network.Name).Info("Auto-seeding external bounds from production")

		if err := seeder.SeedFromProduction(ctx, network.Name, redisDB); err != nil {
			m.log.WithError(err).WithField("network", network.Name).
				Warn("Failed to seed external bounds from production (non-fatal)")

			continue
		}

		m.log.WithField("network", network.Name).Info("External bounds seeded successfully")

		seededCount++
	}

	// Update spinner with final result
	if seededCount > 0 {
		spinner.UpdateText(fmt.Sprintf("Seeded external bounds for %d network(s)", seededCount))
	} else if skippedCount > 0 {
		spinner.UpdateText(fmt.Sprintf("External bounds already exist for %d network(s)", skippedCount))
	}

	return nil
}

// checkPort checks if a port is open.
func (m *Manager) checkPort(ctx context.Context, addr string) bool {
	d := net.Dialer{Timeout: 1 * time.Second}

	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}

	conn.Close()

	return true
}

// checkClusterShardDNS performs DNS lookups for individual shard endpoints
// when the configured host matches a ClickHouse cluster (e.g., chendpoint-clickhouse-raw).
// This helps diagnose connectivity issues to specific shards in the cluster.
func (m *Manager) checkClusterShardDNS(ctx context.Context, host string) error {
	// Extract the first component of the hostname
	dotIdx := strings.Index(host, ".")
	if dotIdx == -1 {
		return nil
	}

	hostPrefix := host[:dotIdx]
	if hostPrefix != clickHouseClusterHostPrefix {
		return nil
	}

	m.log.Info("ClickHouse cluster detected, checking DNS for individual shards")

	domainSuffix := host[dotIdx:]

	var dnsErrors []string

	for _, shard := range clickHouseClusterShards {
		shardHost := fmt.Sprintf("%s-%s%s", clickHouseClusterHostPrefix, shard, domainSuffix)

		_, err := net.LookupHost(shardHost)
		if err != nil {
			ui.Error(fmt.Sprintf("DNS lookup failed for shard %s: %s", shard, shardHost))

			m.log.WithFields(logrus.Fields{
				"shard":      shard,
				logFieldHost: shardHost,
				"error":      err.Error(),
			}).Warn("DNS lookup failed for shard")

			dnsErrors = append(dnsErrors, fmt.Sprintf("%s: %v", shardHost, err))
		} else {
			ui.Success(fmt.Sprintf("DNS OK for shard %s: %s", shard, shardHost))

			m.log.WithFields(logrus.Fields{
				"shard":      shard,
				logFieldHost: shardHost,
			}).Debug("DNS lookup successful for shard")
		}
	}

	if len(dnsErrors) > 0 {
		return fmt.Errorf("DNS lookup failed for %d/%d ClickHouse shards:\n  %s",
			len(dnsErrors), len(clickHouseClusterShards), strings.Join(dnsErrors, "\n  "))
	}

	m.log.Info("all ClickHouse shard DNS lookups successful")

	return nil
}

// migrateXatu runs xatu schema migrations against the local xatu-clickhouse cluster
// by delegating to the xatu-cbt binary, which owns the xatu migration logic
// (database-agnostic per-schema sets applied per target database). This creates the
// external source tables (beacon_api_*, canonical_*, mev_relay_*, etc.) that CBT
// transformations depend on. Only invoked in local mode; an external/remote xatu
// already has its schema.
func (m *Manager) migrateXatu(ctx context.Context) error {
	args := m.infraActionArgs("migrate-xatu")
	if m.cfg.Dev.XatuRef != "" {
		args = append(args, "--xatu-ref", m.cfg.Dev.XatuRef)
	}

	if err := m.runXatuCBT(ctx, args...); err != nil {
		return fmt.Errorf("failed to run xatu migrations: %w", err)
	}

	return nil
}

// startObservability initializes and starts the observability stack.
// It generates the required config files before starting containers.
func (m *Manager) startObservability(ctx context.Context) error {
	// Generate observability configs before starting containers
	// This is done here because infrastructure starts before the main config generation phase
	configsDir := filepath.Join(m.xcliDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		return fmt.Errorf("failed to create configs directory: %w", err)
	}

	generator := configgen.NewGenerator(m.log, m.cfg)
	if m.runtime != nil {
		generator = configgen.NewRuntimeGenerator(m.log, m.runtime)
	}

	m.log.Debug("generating observability configs")

	if _, err := generator.GeneratePrometheusConfig(configsDir); err != nil {
		return fmt.Errorf("failed to generate Prometheus config: %w", err)
	}

	if err := generator.GenerateGrafanaProvisioning(configsDir, m.userStateDir()); err != nil {
		return fmt.Errorf("failed to generate Grafana provisioning: %w", err)
	}

	// Create and start the observability manager
	if m.observability == nil {
		obsMgr, err := NewObservabilityManagerWithRuntime(m.log, m.cfg, m.xcliDir, m.runtime)
		if err != nil {
			return fmt.Errorf("failed to create observability manager: %w", err)
		}

		m.observability = obsMgr
	}

	return m.observability.Start(ctx)
}

// stopObservability stops the observability stack.
func (m *Manager) stopObservability(ctx context.Context) error {
	if m.observability == nil {
		// Create manager just for stopping (in case containers exist from previous run)
		obsMgr, err := NewObservabilityManagerWithRuntime(m.log, m.cfg, m.xcliDir, m.runtime)
		if err != nil {
			return fmt.Errorf("failed to create observability manager: %w", err)
		}

		m.observability = obsMgr
	}

	return m.observability.Stop(ctx)
}
