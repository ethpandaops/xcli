// Package process manages long-running service processes with PID tracking,
// log management, health checking, and graceful shutdown capabilities.
package process

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/sirupsen/logrus"
)

const (
	// gracefulShutdownTimeout is the maximum time to wait for a process to stop gracefully.
	gracefulShutdownTimeout = 30 * time.Second
	// shutdownPollInterval is how often to check if a process has stopped.
	shutdownPollInterval = 100 * time.Millisecond
	pidFileVersion       = 1
)

// Compile-time interface check.
var _ Manager = (*manager)(nil)

// Process represents a managed process.
type Process struct {
	Name    string
	Cmd     *exec.Cmd
	PID     int
	LogFile string
	Started time.Time
}

// manager implements the Manager interface.
type manager struct {
	log       logrus.FieldLogger
	processes map[string]*Process
	stateDir  string
	mu        sync.RWMutex
}

// NewManager creates a new process manager.
func NewManager(log logrus.FieldLogger, stateDir string) Manager {
	m := &manager{
		log:       log.WithField("component", "process-manager"),
		processes: make(map[string]*Process, 10), // Typical: 5-10 services
		stateDir:  stateDir,
	}

	// Load existing PIDs from disk
	m.loadPIDs()

	return m
}

// PIDFileData represents the JSON structure of a persisted PID file
// containing process metadata for crash recovery and monitoring.
type PIDFileData struct {
	Version   int       `json:"version"` // Format version (currently 1)
	PID       int       `json:"pid"`
	LogFile   string    `json:"logFile"`
	Command   string    `json:"command"`   // Binary path
	Args      []string  `json:"args"`      // Command arguments
	StartedAt time.Time `json:"startedAt"` // ISO8601 timestamp
}

// Start starts a new process with optional health checking.
// If healthCheck is nil, uses NoOpHealthChecker (existing behavior).
func (m *manager) Start(ctx context.Context, name string, cmd *exec.Cmd, healthCheck HealthChecker) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already running
	if p, exists := m.processes[name]; exists {
		if m.isRunning(p) {
			return fmt.Errorf("process %s is already running (PID %d)", name, p.PID)
		}
		// Clean up stale entry
		delete(m.processes, name)
	}

	// Setup log file - truncate to start fresh
	logFile := filepath.Join(m.stateDir, constants.DirLogs, fmt.Sprintf(constants.LogFileTemplate, name))
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	logFd, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Setup stdout/stderr - only write to log file to avoid broken pipe when parent exits
	cmd.Stdout = logFd
	cmd.Stderr = logFd

	// Put child in its own process group so it survives parent (CC server) dying
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Start the process
	if err := cmd.Start(); err != nil {
		logFd.Close()

		return fmt.Errorf("failed to start process: %w", err)
	}

	process := &Process{
		Name:    name,
		Cmd:     cmd,
		PID:     cmd.Process.Pid,
		LogFile: logFile,
		Started: time.Now(),
	}

	m.processes[name] = process

	// Save PID to disk (JSON format)
	m.savePID(name, process, cmd)

	// Default to no-op health checker if none provided
	if healthCheck == nil {
		healthCheck = &NoOpHealthChecker{}
	}

	// Run health check after starting
	m.log.WithFields(logrus.Fields{
		"name":         name,
		"pid":          process.PID,
		"health_check": healthCheck.Name(),
	}).Debug("running health check")

	if err := healthCheck.Check(ctx); err != nil {
		// Health check failed - kill the process
		m.log.WithError(err).Warn("health check failed, stopping process")

		if stopErr := m.Stop(ctx, name); stopErr != nil {
			m.log.WithError(stopErr).Warn("failed to stop unhealthy process")
		}

		return fmt.Errorf("health check failed: %w", err)
	}

	m.log.WithField("name", name).Info("process started and healthy")

	// Monitor process in background
	go m.monitor(name, process, logFd)

	return nil
}

// Stop stops a process gracefully.
func (m *manager) Stop(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.processes[name]
	if !exists {
		// Process is already stopped - goal achieved, return success
		return nil
	}

	m.log.WithFields(logrus.Fields{
		"name": name,
		"pid":  p.PID,
	}).Info("stopping process")

	// Get process handle (works for both started and loaded processes)
	process, err := os.FindProcess(p.PID)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// Send SIGTERM to the process group for graceful shutdown.
	// Negative PID signals the entire process group (created via Setpgid).
	if err := syscall.Kill(-p.PID, syscall.SIGTERM); err != nil {
		// Fall back to signaling just the process (e.g. PID-loaded processes
		// from a previous session that weren't started with Setpgid)
		if sigErr := process.Signal(syscall.SIGTERM); sigErr != nil {
			return fmt.Errorf("failed to send SIGTERM: %w", sigErr)
		}
	}

	// Wait for graceful shutdown with context cancellation support
	ticker := time.NewTicker(shutdownPollInterval)
	defer ticker.Stop()

	timeout := time.After(gracefulShutdownTimeout)

	for {
		select {
		case <-ctx.Done():
			// Context cancelled - force kill immediately
			m.log.WithField("name", name).Warn("Context cancelled, sending SIGKILL")

			_ = syscall.Kill(-p.PID, syscall.SIGKILL)

			if err := process.Kill(); err != nil {
				m.log.WithError(err).Warn("failed to kill process")
			}

			delete(m.processes, name)
			m.removePID(name)

			return ctx.Err()
		case <-timeout:
			// Graceful shutdown timeout - force kill
			m.log.WithField("name", name).Warn("Process did not stop gracefully, sending SIGKILL")

			_ = syscall.Kill(-p.PID, syscall.SIGKILL)

			if err := process.Kill(); err != nil {
				return fmt.Errorf("failed to kill process: %w", err)
			}

			delete(m.processes, name)
			m.removePID(name)

			return nil
		case <-ticker.C:
			if err := process.Signal(syscall.Signal(0)); err != nil {
				// Process is gone
				delete(m.processes, name)
				m.removePID(name)

				return nil
			}
		}
	}
}

// StopAll stops all managed processes, including orphaned processes from PID files.
func (m *manager) StopAll(ctx context.Context) error {
	m.log.Info("stopping all managed processes")

	// First, reload PIDs from disk to catch any orphaned processes
	// that weren't in memory (e.g., from previous xcli sessions)
	m.mu.Lock()
	pidDir := filepath.Join(m.stateDir, constants.DirPIDs)

	entries, err := os.ReadDir(pidDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".pid" {
				name := entry.Name()[:len(entry.Name())-4]
				// Only load if not already in memory
				if _, exists := m.processes[name]; !exists {
					m.log.WithField("name", name).Debug(
						"found orphaned PID file, attempting to load",
					)
					m.loadPID(name)
				}
			}
		}
	}

	// Get all process names
	names := make([]string, 0, len(m.processes))
	for name := range m.processes {
		names = append(names, name)
	}

	m.mu.Unlock()

	m.log.WithField("count", len(names)).Info("stopping processes")

	// Stop all processes
	var errs []error

	for _, name := range names {
		if err := m.Stop(ctx, name); err != nil {
			m.log.WithFields(logrus.Fields{
				"name":  name,
				"error": err,
			}).Warn("failed to stop process")
			errs = append(errs, fmt.Errorf("stop %s: %w", name, err))
		}

		// Check for context cancellation between processes
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to stop %d processes: %v", len(errs), errs)
	}

	m.log.Info("all processes stopped successfully")

	return nil
}

// Restart restarts a process.
func (m *manager) Restart(ctx context.Context, name string) error {
	m.mu.RLock()

	p, exists := m.processes[name]
	if !exists {
		m.mu.RUnlock()

		return fmt.Errorf("process %s is not running", name)
	}

	// Check if we have the command info (processes loaded from PID files don't have this)
	if p.Cmd == nil {
		m.mu.RUnlock()

		return fmt.Errorf("cannot restart process %s: loaded from PID file without command info. Stop and restart the entire stack instead", name)
	}

	// Copy the command for restart
	oldCmd := p.Cmd

	m.mu.RUnlock()

	// Stop the process
	if err := m.Stop(ctx, name); err != nil {
		return fmt.Errorf("failed to stop process: %w", err)
	}

	// Wait a bit for cleanup
	time.Sleep(500 * time.Millisecond)

	// Create new command with same args
	//nolint:gosec // Command is from previously validated process
	newCmd := exec.CommandContext(ctx, oldCmd.Path, oldCmd.Args[1:]...)
	newCmd.Dir = oldCmd.Dir
	newCmd.Env = oldCmd.Env

	// Start again (no health check for restart)
	return m.Start(ctx, name, newCmd, nil)
}

// List returns all running processes.
func (m *manager) List() []*Process {
	m.mu.RLock()
	defer m.mu.RUnlock()

	processes := make([]*Process, 0, len(m.processes))
	for _, p := range m.processes {
		processes = append(processes, p)
	}

	return processes
}

// Get returns a specific process.
func (m *manager) Get(name string) (*Process, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, exists := m.processes[name]

	return p, exists
}

// isRunning checks if a process is actually running.
// Handles both manager-started processes (Cmd != nil) and PID-loaded processes (Cmd == nil).
func (m *manager) isRunning(p *Process) bool {
	if p.Cmd != nil && p.Cmd.Process != nil {
		return p.Cmd.Process.Signal(syscall.Signal(0)) == nil
	}

	// PID-loaded process (Cmd is nil) — check via PID directly
	if p.PID <= 0 {
		return false
	}

	proc, err := os.FindProcess(p.PID)
	if err != nil {
		return false
	}

	return proc.Signal(syscall.Signal(0)) == nil
}

// IsRunning checks if a process with the given name is running.
// Public method that implements Manager interface.
func (m *manager) IsRunning(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, exists := m.processes[name]
	if !exists {
		return false
	}

	return m.isRunning(p)
}

// TailLogs tails the log file for a process.
func (m *manager) TailLogs(ctx context.Context, name string, follow bool) error {
	m.mu.RLock()
	p, exists := m.processes[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("process %s is not running", name)
	}

	if follow {
		// Use tail -f
		//nolint:gosec // LogFile is managed internally and validated
		cmd := exec.CommandContext(ctx, "tail", "-f", p.LogFile)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		return cmd.Run()
	}

	// Just cat the file
	data, err := os.ReadFile(p.LogFile)
	if err != nil {
		return fmt.Errorf("failed to read log file: %w", err)
	}

	fmt.Print(string(data))

	return nil
}

// CleanLogs removes all log files.
func (m *manager) CleanLogs() error {
	logsDir := filepath.Join(m.stateDir, constants.DirLogs)

	// Check if logs directory exists
	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		return nil // Nothing to clean
	}

	// Remove all log files
	if err := os.RemoveAll(logsDir); err != nil {
		return fmt.Errorf("failed to remove logs directory: %w", err)
	}

	m.log.Info("cleaned all log files")

	return nil
}

// loadPID loads a single PID from disk (JSON format only).
func (m *manager) loadPID(name string) {
	pidFile := filepath.Join(m.stateDir, constants.DirPIDs, fmt.Sprintf(constants.PIDFileTemplate, name))

	content, err := os.ReadFile(pidFile)
	if err != nil {
		m.log.WithFields(logrus.Fields{
			"name":    name,
			"pidFile": pidFile,
		}).Debug("failed to read PID file")

		return
	}

	var data PIDFileData
	if unmarshalErr := json.Unmarshal(content, &data); unmarshalErr != nil {
		m.log.WithFields(logrus.Fields{
			"name": name,
		}).Warn("failed to parse PID file, removing stale file")
		m.removePID(name)

		return
	}

	if data.Version != pidFileVersion {
		m.log.WithField("version", data.Version).Warn("unknown PID file version")
	}

	// Validate process exists
	process, err := os.FindProcess(data.PID)
	if err != nil {
		m.removePID(name)

		return
	}

	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process doesn't exist - remove stale PID file
		m.removePID(name)

		return
	}

	// Add to processes map (without Cmd since we can't reconstruct it perfectly)
	m.processes[name] = &Process{
		Name:    name,
		Cmd:     nil, // Can't reconstruct
		PID:     data.PID,
		LogFile: data.LogFile,
		Started: data.StartedAt,
	}

	m.log.WithFields(logrus.Fields{
		"name": name,
		"pid":  data.PID,
	}).Debug("loaded process from PID file")
}

// monitor watches a process and cleans up when it exits.
func (m *manager) monitor(name string, p *Process, logFd *os.File) {
	defer logFd.Close()

	err := p.Cmd.Wait()

	m.mu.Lock()
	delete(m.processes, name)
	m.mu.Unlock()

	// Remove PID file
	m.removePID(name)

	if err != nil {
		m.log.WithFields(logrus.Fields{
			"name": name,
			"pid":  p.PID,
			"err":  err,
		}).Warn("Process exited with error")
	} else {
		m.log.WithFields(logrus.Fields{
			"name": name,
			"pid":  p.PID,
		}).Info("Process exited")
	}
}

// savePID saves a process PID to disk in JSON format.
func (m *manager) savePID(name string, p *Process, cmd *exec.Cmd) {
	pidDir := filepath.Join(m.stateDir, constants.DirPIDs)
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		m.log.WithError(err).Warn("failed to create PID directory")

		return
	}

	data := PIDFileData{
		Version:   pidFileVersion,
		PID:       p.PID,
		LogFile:   p.LogFile,
		Command:   cmd.Path,
		Args:      cmd.Args[1:], // Skip binary name
		StartedAt: p.Started,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		m.log.WithError(err).Warn("failed to marshal PID data")

		return
	}

	pidFile := filepath.Join(pidDir, fmt.Sprintf(constants.PIDFileTemplate, name))
	//nolint:gosec // PID file permissions are intentionally 0644 for readability
	if err := os.WriteFile(pidFile, jsonData, 0644); err != nil {
		m.log.WithError(err).Warn("failed to write PID file")
	}
}

// removePID removes a PID file.
func (m *manager) removePID(name string) {
	pidFile := filepath.Join(m.stateDir, constants.DirPIDs, fmt.Sprintf(constants.PIDFileTemplate, name))
	os.Remove(pidFile)
}

// ReloadPIDs re-scans the PID directory for new or changed PID files.
// Safe to call concurrently. Discovers processes started externally (e.g. by `xcli lab up`)
// and removes stale entries for processes that have exited.
func (m *manager) ReloadPIDs() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.loadPIDsLocked()
}

// loadPIDs loads PIDs from disk and checks if processes are still running.
func (m *manager) loadPIDs() {
	m.loadPIDsLocked()
}

// loadPIDsLocked scans the PID directory and updates the process map.
// Caller must hold m.mu (or be in a context where no concurrent access occurs).
func (m *manager) loadPIDsLocked() {
	pidDir := filepath.Join(m.stateDir, constants.DirPIDs)

	entries, err := os.ReadDir(pidDir)
	if err != nil {
		return // Directory doesn't exist or can't be read
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".pid" {
			continue
		}

		name := entry.Name()[:len(entry.Name())-4] // Remove .pid extension

		// Skip processes started by this manager (Cmd != nil) — don't overwrite them
		if existing, exists := m.processes[name]; exists && existing.Cmd != nil {
			continue
		}

		// For existing PID-loaded processes, re-verify they're still alive
		if existing, exists := m.processes[name]; exists && existing.Cmd == nil {
			if !m.isRunning(existing) {
				delete(m.processes, name)
				m.removePID(name)

				m.log.WithField("name", name).Debug("removed stale PID-loaded process")
			}

			continue
		}

		// New PID file not yet in m.processes — load it
		m.loadPID(name)
	}
}
