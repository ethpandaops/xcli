package diagnostic

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ClaudeClient handles communication with Claude Code CLI.
type ClaudeClient struct {
	log        logrus.FieldLogger
	claudePath string
	timeout    time.Duration
}

// AIDiagnosis contains Claude's analysis.
type AIDiagnosis struct {
	RootCause     string   `json:"rootCause"`
	Explanation   string   `json:"explanation"`
	AffectedFiles []string `json:"affectedFiles"`
	Suggestions   []string `json:"suggestions"`
	FixCommands   []string `json:"fixCommands,omitempty"`
}

// NewClaudeClient creates a client, auto-detecting claude binary.
func NewClaudeClient(log logrus.FieldLogger) (*ClaudeClient, error) {
	claudePath, err := findClaudeBinary()
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found: %w", err)
	}

	client := &ClaudeClient{
		log:        log.WithField("component", "claude-client"),
		claudePath: claudePath,
		timeout:    2 * time.Minute, // Claude Code can take a while, especially first run
	}

	return client, nil
}

// IsAvailable checks if Claude Code CLI is installed.
func (c *ClaudeClient) IsAvailable() bool {
	if c.claudePath == "" {
		return false
	}

	// Verify the binary exists and is executable
	info, err := os.Stat(c.claudePath)
	if err != nil {
		return false
	}

	// Check if it's executable (not a directory)
	return !info.IsDir() && info.Mode()&0111 != 0
}

// Diagnose sends error context to Claude for analysis.
func (c *ClaudeClient) Diagnose(ctx context.Context, report *RebuildReport) (*AIDiagnosis, error) {
	if !c.IsAvailable() {
		return nil, fmt.Errorf("claude CLI is not available")
	}

	// Build the diagnostic prompt
	prompt := c.buildPrompt(report)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Execute claude CLI with --print flag for non-interactive mode
	// Input must be provided via stdin when using --print
	//nolint:gosec // claudePath is validated in findClaudeBinary
	cmd := exec.CommandContext(ctx, c.claudePath, "--print")

	// Provide prompt via stdin
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	c.log.WithFields(logrus.Fields{
		"timeout": c.timeout,
		"binary":  c.claudePath,
	}).Debug("Invoking Claude CLI for diagnosis")

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("claude diagnosis timed out after %s", c.timeout)
		}

		return nil, fmt.Errorf("claude CLI failed: %w (stderr: %s)", err, stderr.String())
	}

	response := stdout.String()
	if response == "" {
		return nil, fmt.Errorf("claude returned empty response")
	}

	c.log.WithField("response_length", len(response)).Debug("Received Claude response")

	// Parse the structured response
	diagnosis := c.parseResponse(response)

	return diagnosis, nil
}

// buildPrompt creates the diagnostic prompt for Claude.
func (c *ClaudeClient) buildPrompt(report *RebuildReport) string {
	var sb strings.Builder

	sb.WriteString("Analyze these build errors and provide a diagnosis.\n\n")
	sb.WriteString("## Instructions\n")
	sb.WriteString("You are analyzing build failures from a development environment. ")
	sb.WriteString("Provide a structured analysis with the following sections:\n\n")
	sb.WriteString("1. **## Root Cause** - A single sentence identifying the primary issue\n")
	sb.WriteString("2. **## Explanation** - 2-3 sentences explaining why this error occurred\n")
	sb.WriteString("3. **## Affected Files** - List of files that likely need changes (one per line, prefixed with -)\n")
	sb.WriteString("4. **## Suggestions** - Numbered list of specific actions to fix the issue\n")
	sb.WriteString("5. **## Fix Commands** - Shell commands that might help (prefixed with $)\n\n")
	sb.WriteString("Be specific and actionable. Focus on the root cause, not symptoms.\n\n")

	// Add failed results
	failed := report.Failed()
	if len(failed) == 0 {
		sb.WriteString("## Build Results\nNo failures found.\n")

		return sb.String()
	}

	sb.WriteString("## Failed Build Steps\n\n")

	for i, result := range failed {
		sb.WriteString(fmt.Sprintf("### Failure %d: %s - %s\n\n", i+1, result.Service, result.Phase))
		sb.WriteString(fmt.Sprintf("- **Command**: `%s`\n", sanitizeOutput(result.Command)))
		sb.WriteString(fmt.Sprintf("- **Working Directory**: `%s`\n", sanitizeOutput(result.WorkDir)))
		sb.WriteString(fmt.Sprintf("- **Exit Code**: %d\n", result.ExitCode))
		sb.WriteString(fmt.Sprintf("- **Duration**: %s\n\n", result.Duration.Round(time.Millisecond)))

		// Include stderr (truncated)
		stderr := sanitizeOutput(result.Stderr)
		if stderr != "" {
			if len(stderr) > 1500 {
				stderr = stderr[:1500] + "\n... (truncated)"
			}

			sb.WriteString("**Stderr**:\n```\n")
			sb.WriteString(stderr)
			sb.WriteString("\n```\n\n")
		}

		// Include stdout if stderr is empty or short
		if len(result.Stderr) < 100 && result.Stdout != "" {
			stdout := sanitizeOutput(result.Stdout)
			if len(stdout) > 1500 {
				stdout = stdout[:1500] + "\n... (truncated)"
			}

			sb.WriteString("**Stdout**:\n```\n")
			sb.WriteString(stdout)
			sb.WriteString("\n```\n\n")
		}
	}

	// Add context about successful builds for reference
	succeeded := report.Succeeded()
	if len(succeeded) > 0 {
		sb.WriteString("## Successful Build Steps (for context)\n\n")

		for _, result := range succeeded {
			sb.WriteString(fmt.Sprintf("- %s - %s (%s)\n",
				result.Service, result.Phase, result.Duration.Round(time.Millisecond)))
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// parseResponse extracts structured diagnosis from Claude's response.
func (c *ClaudeClient) parseResponse(response string) *AIDiagnosis {
	diagnosis := &AIDiagnosis{
		AffectedFiles: make([]string, 0),
		Suggestions:   make([]string, 0),
		FixCommands:   make([]string, 0),
	}

	// Define section patterns
	sections := map[string]*string{
		"Root Cause":  &diagnosis.RootCause,
		"Explanation": &diagnosis.Explanation,
	}

	// Extract simple text sections
	for sectionName, target := range sections {
		*target = extractSection(response, sectionName)
	}

	// Extract Affected Files (bullet points)
	affectedFilesSection := extractSection(response, "Affected Files")
	diagnosis.AffectedFiles = extractBulletPoints(affectedFilesSection)

	// Extract Suggestions (numbered or bullet points)
	suggestionsSection := extractSection(response, "Suggestions")
	diagnosis.Suggestions = extractListItems(suggestionsSection)

	// Extract Fix Commands (lines starting with $ or in code blocks)
	fixCommandsSection := extractSection(response, "Fix Commands")
	diagnosis.FixCommands = extractCommands(fixCommandsSection)

	// If structured parsing failed, try to extract what we can
	if diagnosis.RootCause == "" && diagnosis.Explanation == "" {
		// Fall back to using the entire response as explanation
		diagnosis.Explanation = strings.TrimSpace(response)
		diagnosis.RootCause = "See explanation below"
	}

	return diagnosis
}

// extractSection extracts content between a section header and the next section.
func extractSection(text, sectionName string) string {
	// Match section header (## Section Name or **Section Name**)
	patterns := []string{
		fmt.Sprintf(`(?i)##\s*%s\s*\n([\s\S]*?)(?:\n##|\n\*\*[A-Z]|\z)`, regexp.QuoteMeta(sectionName)),
		fmt.Sprintf(`(?i)\*\*%s\*\*:?\s*\n?([\s\S]*?)(?:\n\*\*[A-Z]|\n##|\z)`, regexp.QuoteMeta(sectionName)),
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(text)

		if len(matches) > 1 {
			content := strings.TrimSpace(matches[1])
			// Remove leading/trailing code block markers if present
			content = strings.TrimPrefix(content, "```")
			content = strings.TrimSuffix(content, "```")

			return strings.TrimSpace(content)
		}
	}

	return ""
}

// extractBulletPoints extracts items from a bullet list.
func extractBulletPoints(text string) []string {
	items := make([]string, 0)

	// Match lines starting with -, *, or •
	re := regexp.MustCompile(`(?m)^[\s]*[-*•]\s*(.+)$`)
	matches := re.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		if len(match) > 1 {
			item := strings.TrimSpace(match[1])
			// Remove backticks around file paths
			item = strings.Trim(item, "`")

			if item != "" {
				items = append(items, item)
			}
		}
	}

	return items
}

// extractListItems extracts items from numbered or bullet lists.
func extractListItems(text string) []string {
	items := make([]string, 0)

	// Match numbered items (1. item) or bullet points
	re := regexp.MustCompile(`(?m)^[\s]*(?:\d+[.)]\s*|[-*•]\s*)(.+)$`)
	matches := re.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		if len(match) > 1 {
			item := strings.TrimSpace(match[1])
			if item != "" {
				items = append(items, item)
			}
		}
	}

	return items
}

// extractCommands extracts shell commands from text.
func extractCommands(text string) []string {
	commands := make([]string, 0)

	// Match lines starting with $
	dollarRe := regexp.MustCompile(`(?m)^\s*\$\s*(.+)$`)
	dollarMatches := dollarRe.FindAllStringSubmatch(text, -1)

	for _, match := range dollarMatches {
		if len(match) > 1 {
			cmd := strings.TrimSpace(match[1])
			if cmd != "" {
				commands = append(commands, cmd)
			}
		}
	}

	// Also extract commands from code blocks
	codeBlockRe := regexp.MustCompile("```(?:bash|sh|shell)?\\n([\\s\\S]*?)```")
	codeMatches := codeBlockRe.FindAllStringSubmatch(text, -1)

	for _, match := range codeMatches {
		if len(match) > 1 {
			lines := strings.Split(match[1], "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				// Remove $ prefix if present
				line = strings.TrimPrefix(line, "$ ")

				if line != "" && !strings.HasPrefix(line, "#") {
					commands = append(commands, line)
				}
			}
		}
	}

	return commands
}

// findClaudeBinary locates the claude CLI binary.
func findClaudeBinary() (string, error) {
	// First, try `which claude`
	whichCmd := exec.Command("which", "claude")

	output, err := whichCmd.Output()
	if err == nil {
		path := strings.TrimSpace(string(output))
		if path != "" {
			return path, nil
		}
	}

	// Get home directory for path construction
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}

	// Search in common locations
	searchPaths := []string{
		filepath.Join(home, ".volta", "bin", "claude"),
		"/usr/local/bin/claude",
		filepath.Join(home, "go", "bin", "claude"),
		filepath.Join(home, ".local", "bin", "claude"),
		"/opt/homebrew/bin/claude",
	}

	for _, path := range searchPaths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}

	return "", fmt.Errorf("claude binary not found in PATH or common locations")
}

// sanitizeOutput removes sensitive information.
func sanitizeOutput(output string) string {
	if output == "" {
		return output
	}

	result := output

	// Replace home directory paths with ~
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		// Handle /Users/xxx/ pattern (macOS)
		result = strings.ReplaceAll(result, home, "~")
	}

	// Replace /home/xxx/ patterns (Linux)
	homePattern := regexp.MustCompile(`/home/[^/\s]+/`)
	result = homePattern.ReplaceAllString(result, "~/")

	// Replace /Users/xxx/ patterns (macOS, for other users)
	usersPattern := regexp.MustCompile(`/Users/[^/\s]+/`)
	result = usersPattern.ReplaceAllString(result, "~/")

	// Remove strings that look like tokens (32+ alphanumeric chars, often with mixed case)
	// This catches API keys, tokens, secrets, etc.
	tokenPattern := regexp.MustCompile(`[A-Za-z0-9_-]{32,}`)
	result = tokenPattern.ReplaceAllString(result, "[REDACTED]")

	// Remove common secret patterns
	secretPatterns := []struct {
		pattern *regexp.Regexp
		replace string
	}{
		{regexp.MustCompile(`(?i)(API_KEY|APIKEY|API-KEY)\s*[=:]\s*[^\s]+`), "$1=[REDACTED]"},
		{regexp.MustCompile(`(?i)(TOKEN|AUTH_TOKEN|ACCESS_TOKEN)\s*[=:]\s*[^\s]+`), "$1=[REDACTED]"},
		{regexp.MustCompile(`(?i)(SECRET|SECRET_KEY|PRIVATE_KEY)\s*[=:]\s*[^\s]+`), "$1=[REDACTED]"},
		{regexp.MustCompile(`(?i)(PASSWORD|PASSWD|PWD)\s*[=:]\s*[^\s]+`), "$1=[REDACTED]"},
		{regexp.MustCompile(`(?i)(CREDENTIAL|CRED)\s*[=:]\s*[^\s]+`), "$1=[REDACTED]"},
		{regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]+`), "bearer [REDACTED]"},
		{regexp.MustCompile(`(?i)authorization:\s*[^\n]+`), "authorization: [REDACTED]"},
	}

	for _, sp := range secretPatterns {
		result = sp.pattern.ReplaceAllString(result, sp.replace)
	}

	return result
}
