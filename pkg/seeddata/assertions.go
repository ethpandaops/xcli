package seeddata

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
	"gopkg.in/yaml.v3"
)

// AssertionCheck represents a single assertion check within an assertion.
type AssertionCheck struct {
	Type   string `yaml:"type"`   // greater_than, less_than, equals, etc.
	Column string `yaml:"column"` // Column name to check
	Value  any    `yaml:"value"`  // Value to compare against
}

// Assertion represents a test assertion for a transformation model.
type Assertion struct {
	Name       string           `yaml:"name"`
	SQL        string           `yaml:"sql"`
	Assertions []AssertionCheck `yaml:"assertions,omitempty"` // For dynamic assertions
	Expected   map[string]any   `yaml:"expected,omitempty"`   // For exact value assertions
}

// ClaudeAssertionClient handles assertion generation using Claude CLI.
type ClaudeAssertionClient struct {
	log        logrus.FieldLogger
	claudePath string
	timeout    time.Duration
}

// NewClaudeAssertionClient creates a new Claude client for assertion generation.
func NewClaudeAssertionClient(log logrus.FieldLogger) (*ClaudeAssertionClient, error) {
	claudePath, err := findClaudeBinaryPath()
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found: %w", err)
	}

	return &ClaudeAssertionClient{
		log:        log.WithField("component", "claude-assertions"),
		claudePath: claudePath,
		timeout:    3 * time.Minute, // Assertion generation can take time
	}, nil
}

// IsAvailable checks if Claude Code CLI is installed and available.
func (c *ClaudeAssertionClient) IsAvailable() bool {
	if c.claudePath == "" {
		return false
	}

	info, err := os.Stat(c.claudePath)
	if err != nil {
		return false
	}

	return !info.IsDir() && info.Mode()&0111 != 0
}

// GenerateAssertions uses Claude to analyze transformation SQL and suggest assertions.
func (c *ClaudeAssertionClient) GenerateAssertions(ctx context.Context, transformationSQL string, externalModels []string, modelName string) ([]Assertion, error) {
	if !c.IsAvailable() {
		return nil, fmt.Errorf("claude CLI is not available")
	}

	prompt := c.buildAssertionPrompt(transformationSQL, externalModels, modelName)

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	//nolint:gosec // claudePath is validated in findClaudeBinaryPath
	cmd := exec.CommandContext(ctx, c.claudePath, "--print")
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	c.log.WithFields(logrus.Fields{
		"timeout": c.timeout,
		"model":   modelName,
	}).Debug("invoking Claude CLI for assertion generation")

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("claude assertion generation timed out after %s", c.timeout)
		}

		return nil, fmt.Errorf("claude CLI failed: %w (stderr: %s)", err, stderr.String())
	}

	response := stdout.String()
	if response == "" {
		return nil, fmt.Errorf("claude returned empty response")
	}

	c.log.WithField("response_length", len(response)).Debug("received Claude response")

	return c.parseAssertionResponse(response, modelName)
}

// GetDefaultAssertions returns basic default assertions when Claude is not available.
func GetDefaultAssertions(modelName string) []Assertion {
	return []Assertion{
		{
			Name: "Row count should be greater than zero",
			SQL:  fmt.Sprintf(`SELECT COUNT(*) AS count FROM %s FINAL`, modelName),
			Assertions: []AssertionCheck{
				{Type: "greater_than", Column: "count", Value: 0},
			},
		},
	}
}

// buildAssertionPrompt creates the prompt for Claude to generate assertions.
func (c *ClaudeAssertionClient) buildAssertionPrompt(transformationSQL string, externalModels []string, modelName string) string {
	var sb strings.Builder

	sb.WriteString("Generate test assertions for this ClickHouse transformation model.\n\n")
	sb.WriteString("## Instructions\n")
	sb.WriteString("You are analyzing a ClickHouse transformation SQL to generate test assertions. ")
	sb.WriteString("Output ONLY valid YAML that can be directly parsed. No explanations or markdown code blocks.\n\n")
	sb.WriteString("Generate assertions that verify:\n")
	sb.WriteString("1. Row count is greater than zero\n")
	sb.WriteString("2. Key columns have valid ranges based on the SQL logic\n")
	sb.WriteString("3. Aggregations are mathematically correct\n")
	sb.WriteString("4. No data quality issues (nulls where unexpected, negative values for counts, etc.)\n\n")

	sb.WriteString("## Output Format\n")
	sb.WriteString("Output assertions as a YAML list. Each assertion must have:\n")
	sb.WriteString("- name: descriptive name\n")
	sb.WriteString("- sql: the query (use `")
	sb.WriteString(modelName)
	sb.WriteString(" FINAL` for the table name)\n")
	sb.WriteString("- assertions: list of checks with type, column, value\n\n")

	sb.WriteString("Valid assertion types: greater_than, less_than, greater_than_or_equal, less_than_or_equal, equals\n\n")

	sb.WriteString("Example output format:\n")
	sb.WriteString("- name: Row count should be greater than zero\n")
	sb.WriteString("  sql: |\n")
	sb.WriteString("    SELECT COUNT(*) AS count FROM ")
	sb.WriteString(modelName)
	sb.WriteString(" FINAL\n")
	sb.WriteString("  assertions:\n")
	sb.WriteString("    - type: greater_than\n")
	sb.WriteString("      column: count\n")
	sb.WriteString("      value: 0\n\n")

	sb.WriteString("## Transformation Model: ")
	sb.WriteString(modelName)
	sb.WriteString("\n\n")

	sb.WriteString("## External Dependencies\n")

	for _, model := range externalModels {
		sb.WriteString("- ")
		sb.WriteString(model)
		sb.WriteString("\n")
	}

	sb.WriteString("\n## Transformation SQL\n```sql\n")
	sb.WriteString(transformationSQL)
	sb.WriteString("\n```\n\n")

	sb.WriteString("Generate 5-10 meaningful assertions. Output ONLY the YAML list, no other text.\n")

	return sb.String()
}

// parseAssertionResponse parses Claude's YAML response into assertions.
func (c *ClaudeAssertionClient) parseAssertionResponse(response, modelName string) ([]Assertion, error) {
	// Try to extract YAML from the response
	yamlContent := extractYAMLFromResponse(response)
	if yamlContent == "" {
		return nil, fmt.Errorf("no valid YAML found in Claude response")
	}

	var assertions []Assertion

	if unmarshalErr := yaml.Unmarshal([]byte(yamlContent), &assertions); unmarshalErr != nil {
		// Try wrapping in a list if it failed
		if !strings.HasPrefix(strings.TrimSpace(yamlContent), "-") {
			yamlContent = "- " + strings.ReplaceAll(yamlContent, "\n", "\n  ")

			if retryErr := yaml.Unmarshal([]byte(yamlContent), &assertions); retryErr != nil {
				return nil, fmt.Errorf("failed to parse assertions YAML: %w", retryErr)
			}
		} else {
			return nil, fmt.Errorf("failed to parse assertions YAML: %w", unmarshalErr)
		}
	}

	// Validate and clean up assertions
	validAssertions := make([]Assertion, 0, len(assertions))

	for _, a := range assertions {
		if a.Name == "" || a.SQL == "" {
			continue
		}

		// Ensure table name uses FINAL
		if !strings.Contains(a.SQL, "FINAL") {
			a.SQL = strings.ReplaceAll(a.SQL, modelName, modelName+" FINAL")
		}

		validAssertions = append(validAssertions, a)
	}

	if len(validAssertions) == 0 {
		return nil, fmt.Errorf("no valid assertions parsed from Claude response")
	}

	return validAssertions, nil
}

// extractYAMLFromResponse extracts YAML content from Claude's response.
func extractYAMLFromResponse(response string) string {
	// First, try to find YAML in code blocks
	codeBlockRe := regexp.MustCompile("```(?:yaml|yml)?\\n([\\s\\S]*?)```")
	matches := codeBlockRe.FindStringSubmatch(response)

	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// If no code block, look for content starting with a YAML list or discovery YAML
	lines := strings.Split(response, "\n")

	var yamlLines []string

	inYAML := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Start YAML when we see assertion-style or discovery-style start markers
		// Include common typos/variations Claude might output
		if strings.HasPrefix(trimmed, "- name:") ||
			strings.HasPrefix(trimmed, "primaryRangeType:") ||
			strings.HasPrefix(trimmed, "primaryrangeType:") ||
			strings.HasPrefix(trimmed, "primary_range_type:") {
			inYAML = true
		}

		if inYAML {
			// Stop if we hit non-YAML content (text that doesn't look like YAML)
			if trimmed != "" &&
				!strings.HasPrefix(line, " ") &&
				!strings.HasPrefix(line, "-") &&
				!strings.HasPrefix(line, "\t") &&
				!strings.Contains(line, ":") {
				break
			}

			yamlLines = append(yamlLines, line)
		}
	}

	if len(yamlLines) > 0 {
		return strings.Join(yamlLines, "\n")
	}

	// Last resort: return trimmed response hoping it's valid YAML
	return strings.TrimSpace(response)
}

// findClaudeBinaryPath locates the claude CLI binary.
func findClaudeBinaryPath() (string, error) {
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
		if info, err := os.Stat(path); err == nil && !info.IsDir() { //nolint:gosec // paths are hardcoded search locations, not user input
			return path, nil
		}
	}

	return "", fmt.Errorf("claude binary not found in PATH or common locations")
}
