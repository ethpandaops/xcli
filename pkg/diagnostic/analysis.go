package diagnostic

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ethpandaops/xcli/pkg/ai"
)

// AIDiagnosis is a backward-compatible alias for the shared diagnosis report type.
type AIDiagnosis = ai.DiagnosisReport

// DiagnosisReport is the generic structured diagnosis payload.
type DiagnosisReport = ai.DiagnosisReport

// BuildReportPrompt creates a diagnosis prompt from a rebuild report.
func BuildReportPrompt(report *RebuildReport) string {
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

	failed := report.Failed()
	if len(failed) == 0 {
		sb.WriteString("## Build Results\nNo failures found.\n")

		return sb.String()
	}

	sb.WriteString("## Failed Build Steps\n\n")

	for i, result := range failed {
		fmt.Fprintf(&sb, "### Failure %d: %s - %s\n\n", i+1, result.Service, result.Phase)
		fmt.Fprintf(&sb, "- **Command**: `%s`\n", sanitizeOutput(result.Command))
		fmt.Fprintf(&sb, "- **Working Directory**: `%s`\n", sanitizeOutput(result.WorkDir))
		fmt.Fprintf(&sb, "- **Exit Code**: %d\n", result.ExitCode)
		fmt.Fprintf(&sb, "- **Duration**: %s\n\n", result.Duration.Round(time.Millisecond))

		stderr := sanitizeOutput(result.Stderr)
		if stderr != "" {
			if len(stderr) > 1500 {
				stderr = stderr[:1500] + "\n... (truncated)"
			}

			sb.WriteString("**Stderr**:\n```\n")
			sb.WriteString(stderr)
			sb.WriteString("\n```\n\n")
		}

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

	succeeded := report.Succeeded()
	if len(succeeded) > 0 {
		sb.WriteString("## Successful Build Steps (for context)\n\n")

		for _, result := range succeeded {
			fmt.Fprintf(&sb, "- %s - %s (%s)\n", result.Service, result.Phase, result.Duration.Round(time.Millisecond))
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// ParseDiagnosisResponse extracts a structured diagnosis from provider output.
func ParseDiagnosisResponse(response string) *DiagnosisReport {
	diagnosis := &DiagnosisReport{
		AffectedFiles: make([]string, 0),
		Suggestions:   make([]string, 0),
		FixCommands:   make([]string, 0),
	}

	sections := map[string]*string{
		"Root Cause":  &diagnosis.RootCause,
		"Explanation": &diagnosis.Explanation,
	}

	for sectionName, target := range sections {
		*target = extractSection(response, sectionName)
	}

	affectedFilesSection := extractSection(response, "Affected Files")
	diagnosis.AffectedFiles = extractBulletPoints(affectedFilesSection)

	suggestionsSection := extractSection(response, "Suggestions")
	diagnosis.Suggestions = extractListItems(suggestionsSection)

	fixCommandsSection := extractSection(response, "Fix Commands")
	diagnosis.FixCommands = extractCommands(fixCommandsSection)

	if diagnosis.RootCause == "" && diagnosis.Explanation == "" {
		diagnosis.Explanation = strings.TrimSpace(response)
		diagnosis.RootCause = "See explanation below"
	}

	return diagnosis
}

// ParseResponse is kept for backward compatibility.
func ParseResponse(response string) *DiagnosisReport {
	return ParseDiagnosisResponse(response)
}

// extractSection extracts content between a section header and the next section.
func extractSection(text, sectionName string) string {
	patterns := []string{
		fmt.Sprintf(`(?i)##\s*%s\s*\n([\s\S]*?)(?:\n##|\n\*\*[A-Z]|\z)`, regexp.QuoteMeta(sectionName)),
		fmt.Sprintf(`(?i)\*\*%s\*\*:?:?\s*\n?([\s\S]*?)(?:\n\*\*[A-Z]|\n##|\z)`, regexp.QuoteMeta(sectionName)),
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)

		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			content := strings.TrimSpace(matches[1])
			content = strings.TrimPrefix(content, "```")
			content = strings.TrimSuffix(content, "```")

			return strings.TrimSpace(content)
		}
	}

	return ""
}

func extractBulletPoints(text string) []string {
	items := make([]string, 0, 8)

	re := regexp.MustCompile(`(?m)^[\s]*[-*•]\s*(.+)$`)
	matches := re.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		if len(match) > 1 {
			item := strings.TrimSpace(match[1])

			item = strings.Trim(item, "`")
			if item != "" {
				items = append(items, item)
			}
		}
	}

	return items
}

func extractListItems(text string) []string {
	items := make([]string, 0, 8)

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

func extractCommands(text string) []string {
	commands := make([]string, 0, 4)

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

	codeBlockRe := regexp.MustCompile("```(?:bash|sh|shell)?\\n([\\s\\S]*?)```")
	codeMatches := codeBlockRe.FindAllStringSubmatch(text, -1)

	for _, match := range codeMatches {
		if len(match) > 1 {
			for _, line := range strings.Split(match[1], "\n") {
				line = strings.TrimSpace(line)

				line = strings.TrimPrefix(line, "$ ")
				if line != "" && !strings.HasPrefix(line, "#") {
					commands = append(commands, line)
				}
			}
		}
	}

	return commands
}

// sanitizeOutput removes sensitive information.
func sanitizeOutput(output string) string {
	if output == "" {
		return output
	}

	result := output

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		result = strings.ReplaceAll(result, home, "~")
	}

	homePattern := regexp.MustCompile(`/home/[^/\s]+/`)
	result = homePattern.ReplaceAllString(result, "~/")

	usersPattern := regexp.MustCompile(`/Users/[^/\s]+/`)
	result = usersPattern.ReplaceAllString(result, "~/")

	tokenPattern := regexp.MustCompile(`[A-Za-z0-9_-]{32,}`)
	result = tokenPattern.ReplaceAllString(result, "[REDACTED]")

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
