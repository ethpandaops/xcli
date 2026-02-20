package cc

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethpandaops/xcli/pkg/diagnostic"
)

// handleGetDiagnoseAvailable returns whether the claude CLI binary is found.
func (a *apiHandler) handleGetDiagnoseAvailable(
	w http.ResponseWriter,
	_ *http.Request,
) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"available": a.claude != nil && a.claude.IsAvailable(),
	})
}

// handlePostDiagnose collects recent logs for a service and sends them
// to Claude CLI for structured diagnosis.
func (a *apiHandler) handlePostDiagnose(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "service name required",
		})

		return
	}

	if a.claude == nil || !a.claude.IsAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "claude CLI is not available",
		})

		return
	}

	// Collect service status info
	var status, health string

	for _, svc := range a.getServicesData() {
		if svc.Name == name {
			status = svc.Status
			health = svc.Health

			break
		}
	}

	if status == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "service not found: " + name,
		})

		return
	}

	// Collect recent logs: disk first, then fall back to in-memory ring buffer
	logLines := a.collectServiceLogs(name)
	if len(logLines) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "no logs available for service: " + name,
		})

		return
	}

	prompt := buildDiagnosePrompt(name, status, health, logLines)

	response, err := a.claude.Ask(r.Context(), prompt)
	if err != nil {
		a.log.WithError(err).WithField("service", name).Error(
			"Claude diagnosis failed",
		)

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("diagnosis failed: %v", err),
		})

		return
	}

	diagnosis := diagnostic.ParseResponse(response)

	writeJSON(w, http.StatusOK, diagnosis)
}

// collectServiceLogs gathers recent log lines for a service from disk
// (last ~300 lines) with a fallback to the in-memory ring buffer.
func (a *apiHandler) collectServiceLogs(name string) []string {
	// Try reading from the log file on disk
	logPath := filepath.Clean(a.orch.LogFilePath(name))

	lines := readLastLines(logPath, 300)
	if len(lines) > 0 {
		return lines
	}

	// Fall back to in-memory log history from the stack context
	// (populated via SSE broadcast loop — covers Docker services)
	if a.logHistoryFn != nil {
		return a.logHistoryFn(name)
	}

	return nil
}

// buildDiagnosePrompt creates the log-focused prompt for Claude.
func buildDiagnosePrompt(
	name, status, health string,
	logLines []string,
) string {
	var sb strings.Builder

	sb.WriteString("Analyze these service logs and provide a diagnosis.\n\n")
	sb.WriteString("## Instructions\n")
	sb.WriteString("You are analyzing logs from a local Ethereum data pipeline service. ")
	sb.WriteString("Provide a structured analysis with the following sections:\n\n")
	sb.WriteString("1. **## Root Cause** - A single sentence identifying the primary issue\n")
	sb.WriteString("2. **## Explanation** - 2-3 sentences explaining why this error occurred\n")
	sb.WriteString("3. **## Affected Files** - List of files that likely need changes (one per line, prefixed with -)\n")
	sb.WriteString("4. **## Suggestions** - Numbered list of specific actions to fix the issue\n")
	sb.WriteString("5. **## Fix Commands** - Shell commands that might help (prefixed with $)\n\n")
	sb.WriteString("Be specific and actionable. Focus on the root cause, not symptoms.\n\n")

	sb.WriteString("## Service Info\n\n")
	//nolint:gosec // name, status, health are from internal service definitions, not user input
	fmt.Fprintf(&sb, "- **Service**: %s\n- **Status**: %s\n- **Health**: %s\n\n", name, status, health)

	sb.WriteString("## Recent Logs\n\n```\n")

	for _, line := range logLines {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString("```\n")

	return sb.String()
}

// readLastLines reads the last n lines from a file.
func readLastLines(path string, n int) []string {
	f, err := os.Open(path) //nolint:gosec // path is constructed by LogFilePath from internal config
	if err != nil {
		return nil
	}
	defer f.Close()

	var all []string

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		all = append(all, scanner.Text())
	}

	if len(all) <= n {
		return all
	}

	return all[len(all)-n:]
}
