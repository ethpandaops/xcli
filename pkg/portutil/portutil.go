package portutil

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

// PortConflict represents a port that's already in use.
type PortConflict struct {
	Port    int
	Service string
	PID     int
	Process string
}

// CheckPort checks if a port is available.
// Returns nil if available, or a PortConflict if the port is in use.
func CheckPort(port int) *PortConflict {
	// Try to listen on the port
	addr := fmt.Sprintf(":%d", port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// Port is in use, try to find what's using it
		return findPortOwner(port)
	}

	// Port is available
	listener.Close()

	return nil
}

// CheckPorts checks multiple ports and returns all conflicts.
func CheckPorts(ports []int) []PortConflict {
	var conflicts []PortConflict

	for _, port := range ports {
		if conflict := CheckPort(port); conflict != nil {
			conflicts = append(conflicts, *conflict)
		}
	}

	return conflicts
}

// findPortOwner tries to find the process using a port.
func findPortOwner(port int) *PortConflict {
	conflict := &PortConflict{
		Port: port,
	}

	// Try lsof (macOS/Linux)
	//nolint:gosec // Port number is validated to be an integer
	cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port), "-sTCP:LISTEN", "-t")

	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		pidStr := strings.TrimSpace(string(output))
		if pid, err := strconv.Atoi(pidStr); err == nil {
			conflict.PID = pid
			// Get process name
			psCmd := exec.Command("ps", "-p", pidStr, "-o", "comm=")
			if psOutput, err := psCmd.Output(); err == nil {
				conflict.Process = strings.TrimSpace(string(psOutput))
			}
		}
	}

	return conflict
}

// FormatConflicts formats port conflicts for display.
func FormatConflicts(conflicts []PortConflict) string {
	if len(conflicts) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Port conflicts detected:\n")

	for _, c := range conflicts {
		sb.WriteString(fmt.Sprintf("  Port %d: in use", c.Port))

		if c.PID > 0 {
			sb.WriteString(fmt.Sprintf(" by PID %d", c.PID))

			if c.Process != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", c.Process))
			}
		}

		sb.WriteString("\n")
	}

	sb.WriteString("\nTo fix this:\n")
	sb.WriteString("  1. Run 'xcli lab down' to clean up\n")

	if len(conflicts) > 0 && conflicts[0].PID > 0 {
		sb.WriteString("  2. Or manually kill processes: ")

		pids := make([]string, 0, len(conflicts))
		for _, c := range conflicts {
			if c.PID > 0 {
				pids = append(pids, strconv.Itoa(c.PID))
			}
		}

		if len(pids) > 0 {
			sb.WriteString(fmt.Sprintf("kill %s\n", strings.Join(pids, " ")))
		}
	}

	return sb.String()
}

// KillProcess attempts to kill a process by PID.
// Sends SIGTERM first, then SIGKILL if needed.
func KillProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}

	// Use kill command for cross-platform compatibility
	//nolint:gosec // PID is validated to be a positive integer
	cmd := exec.Command("kill", strconv.Itoa(pid))
	if err := cmd.Run(); err != nil {
		// Try force kill if graceful kill failed
		//nolint:gosec // PID is validated to be a positive integer
		forceCmd := exec.Command("kill", "-9", strconv.Itoa(pid))

		return forceCmd.Run()
	}

	return nil
}
