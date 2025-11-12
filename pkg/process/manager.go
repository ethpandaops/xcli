package process

import (
	"context"
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

// Process represents a managed process.
type Process struct {
	Name    string
	Cmd     *exec.Cmd
	PID     int
	LogFile string
	Started time.Time
}

// Manager manages service processes.
type Manager struct {
	log       logrus.FieldLogger
	processes map[string]*Process
	stateDir  string
	mu        sync.RWMutex
}

// NewManager creates a new process manager.
func NewManager(log logrus.FieldLogger, stateDir string) *Manager {
	m := &Manager{
		log:       log.WithField("component", "process-manager"),
		processes: make(map[string]*Process),
		stateDir:  stateDir,
	}

	// Load existing PIDs from disk
	m.loadPIDs()

	return m
}

// Start starts a process.
func (m *Manager) Start(ctx context.Context, name string, cmd *exec.Cmd) error {
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

	// Save PID to disk
	m.savePID(name, process.PID, process.LogFile)

	m.log.WithFields(logrus.Fields{
		"name": name,
		"pid":  process.PID,
	}).Info("process started")

	// Monitor process in background
	go m.monitor(name, process, logFd)

	return nil
}

// monitor watches a process and cleans up when it exits.
func (m *Manager) monitor(name string, p *Process, logFd *os.File) {
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

// Stop stops a process gracefully.
func (m *Manager) Stop(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.processes[name]
	if !exists {
		return fmt.Errorf("process %s is not running", name)
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

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait up to 30 seconds for graceful shutdown
	// Poll every 100ms to check if process is gone
	for i := 0; i < 300; i++ {
		time.Sleep(100 * time.Millisecond)

		if err := process.Signal(syscall.Signal(0)); err != nil {
			// Process is gone
			delete(m.processes, name)
			m.removePID(name)

			return nil
		}
	}

	// Force kill if not stopped
	m.log.WithField("name", name).Warn("Process did not stop gracefully, sending SIGKILL")

	if err := process.Kill(); err != nil {
		return fmt.Errorf("failed to kill process: %w", err)
	}

	delete(m.processes, name)
	m.removePID(name)

	return nil
}

// StopAll stops all managed processes, including orphaned processes from PID files.
func (m *Manager) StopAll() error {
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
		if err := m.Stop(name); err != nil {
			m.log.WithFields(logrus.Fields{
				"name":  name,
				"error": err,
			}).Warn("failed to stop process")
			errs = append(errs, fmt.Errorf("stop %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to stop %d processes: %v", len(errs), errs)
	}

	m.log.Info("all processes stopped successfully")

	return nil
}

// Restart restarts a process.
func (m *Manager) Restart(ctx context.Context, name string) error {
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
	if err := m.Stop(name); err != nil {
		return fmt.Errorf("failed to stop process: %w", err)
	}

	// Wait a bit for cleanup
	time.Sleep(500 * time.Millisecond)

	// Create new command with same args
	//nolint:gosec // Command is from previously validated process
	newCmd := exec.CommandContext(ctx, oldCmd.Path, oldCmd.Args[1:]...)
	newCmd.Dir = oldCmd.Dir
	newCmd.Env = oldCmd.Env

	// Start again
	return m.Start(ctx, name, newCmd)
}

// List returns all running processes.
func (m *Manager) List() []*Process {
	m.mu.RLock()
	defer m.mu.RUnlock()

	processes := make([]*Process, 0, len(m.processes))
	for _, p := range m.processes {
		processes = append(processes, p)
	}

	return processes
}

// Get returns a specific process.
func (m *Manager) Get(name string) (*Process, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, exists := m.processes[name]

	return p, exists
}

// isRunning checks if a process is actually running.
func (m *Manager) isRunning(p *Process) bool {
	if p.Cmd == nil || p.Cmd.Process == nil {
		return false
	}

	// Try to send signal 0 (doesn't actually send signal, just checks)
	err := p.Cmd.Process.Signal(syscall.Signal(0))

	return err == nil
}

// TailLogs tails the log file for a process.
func (m *Manager) TailLogs(ctx context.Context, name string, follow bool) error {
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

// savePID saves a process PID to disk.
func (m *Manager) savePID(name string, pid int, logFile string) {
	pidDir := filepath.Join(m.stateDir, constants.DirPIDs)
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		m.log.WithError(err).Warn("Failed to create PID directory")

		return
	}

	pidFile := filepath.Join(pidDir, fmt.Sprintf(constants.PIDFileTemplate, name))
	data := fmt.Sprintf("%d\n%s\n", pid, logFile)
	//nolint:gosec // PID file permissions are intentionally 0644 for readability
	if err := os.WriteFile(pidFile, []byte(data), 0644); err != nil {
		m.log.WithError(err).Warn("Failed to write PID file")
	}
}

// removePID removes a PID file.
func (m *Manager) removePID(name string) {
	pidFile := filepath.Join(m.stateDir, constants.DirPIDs, fmt.Sprintf(constants.PIDFileTemplate, name))
	os.Remove(pidFile)
}

// loadPIDs loads PIDs from disk and checks if processes are still running.
func (m *Manager) loadPIDs() {
	pidDir := filepath.Join(m.stateDir, constants.DirPIDs)

	entries, err := os.ReadDir(pidDir)
	if err != nil {
		return // Directory doesn't exist or can't be read
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".pid" {
			name := entry.Name()[:len(entry.Name())-4] // Remove .pid extension
			m.loadPID(name)
		}
	}
}

// CleanLogs removes all log files.
func (m *Manager) CleanLogs() error {
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

// loadPID loads a single PID from disk.
func (m *Manager) loadPID(name string) {
	pidFile := filepath.Join(m.stateDir, constants.DirPIDs, fmt.Sprintf(constants.PIDFileTemplate, name))

	data, err := os.ReadFile(pidFile)
	if err != nil {
		m.log.WithFields(logrus.Fields{
			"name":    name,
			"pidFile": pidFile,
		}).Debug("failed to read PID file")

		return
	}

	// Parse PID file: format is "PID\nLOGFILE\n"
	content := string(data)

	// Split by newline and filter empty lines
	var (
		pid     int
		logFile string
	)

	_, err = fmt.Sscanf(content, "%d\n%s\n", &pid, &logFile)
	if err != nil {
		m.log.WithFields(logrus.Fields{
			"name":    name,
			"pidFile": pidFile,
			"content": content,
		}).Warn("failed to parse PID file, removing")
		m.removePID(name)

		return
	}

	// Validate PID
	if pid <= 0 {
		m.log.WithFields(logrus.Fields{
			"name": name,
			"pid":  pid,
		}).Warn("invalid PID in file, removing")
		m.removePID(name)

		return
	}

	// Check if process is still running
	process, err := os.FindProcess(pid)
	if err != nil {
		m.log.WithFields(logrus.Fields{
			"name": name,
			"pid":  pid,
		}).Debug("failed to find process, removing PID file")
		m.removePID(name)

		return
	}

	// Try to signal the process (signal 0 doesn't actually send a signal)
	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process not running
		m.log.WithFields(logrus.Fields{
			"name": name,
			"pid":  pid,
		}).Debug("process not running, removing PID file")
		m.removePID(name)

		return
	}

	// Process is running, add to our map
	m.processes[name] = &Process{
		Name:    name,
		PID:     pid,
		LogFile: logFile,
		Started: time.Now(), // We don't know the real start time
	}

	m.log.WithFields(logrus.Fields{
		"name": name,
		"pid":  pid,
	}).Debug("loaded existing process from PID file")
}
