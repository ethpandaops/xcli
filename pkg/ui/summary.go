package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/pterm/pterm"

	"github.com/ethpandaops/xcli/pkg/diagnostic"
)

const (
	// Box drawing characters.
	boxTopLeft     = "┌"
	boxTopRight    = "┐"
	boxBottomLeft  = "└"
	boxBottomRight = "┘"
	boxHorizontal  = "─"
	boxVertical    = "│"
	boxLeftT       = "├"
	boxRightT      = "┤"

	// Status symbols.
	symbolSuccess = "✓"
	symbolFailure = "✗"
	symbolSkipped = "○"

	// Default box width.
	defaultBoxWidth = 60

	// Output limits.
	maxFailureOutputLines = 20
	maxRawErrorLines      = 30
)

// DisplayBuildSummary shows a rich summary of rebuild results in a formatted box.
// Output format:
// ┌─────────────────────────────────────────────────────────┐
// │  Rebuild Summary                                        │
// ├─────────────────────────────────────────────────────────┤
// │  ✓ xatu-cbt proto-gen     0.3s                         │
// │  ✓ xatu-cbt build         2.1s                         │
// │  ✗ cbt-api proto-gen      0.8s  ← FAILED               │
// │  ○ cbt-api build          -     (skipped)              │
// │  ○ lab-backend build      -     (skipped)              │
// └─────────────────────────────────────────────────────────┘.
func DisplayBuildSummary(report *diagnostic.RebuildReport) {
	if report == nil || len(report.Results) == 0 {
		return
	}

	// Build the box content.
	boxWidth := defaultBoxWidth
	contentWidth := boxWidth - 4 // Account for "│  " and " │"

	// Print top border.
	fmt.Println(boxTopLeft + strings.Repeat(boxHorizontal, boxWidth-2) + boxTopRight)

	// Print header.
	header := "Rebuild Summary"
	headerPadding := contentWidth - len(header)
	fmt.Printf("%s  %s%s %s\n", boxVertical, pterm.Bold.Sprint(header), strings.Repeat(" ", headerPadding), boxVertical)

	// Print separator.
	fmt.Println(boxLeftT + strings.Repeat(boxHorizontal, boxWidth-2) + boxRightT)

	// Track which services have failed to mark subsequent phases as skipped.
	failedServices := make(map[string]bool, len(report.Results))

	// First pass: identify failed services.
	for _, result := range report.Results {
		if !result.Success {
			failedServices[result.Service] = true
		}
	}

	// Second pass: display results with skipped detection.
	// Group results by service to detect skipped phases.
	servicePhases := make(map[string][]diagnostic.BuildResult, len(report.Results))

	for _, result := range report.Results {
		servicePhases[result.Service] = append(servicePhases[result.Service], result)
	}

	// Display each result.
	for _, result := range report.Results {
		var symbol, status, duration string

		isSkipped := isResultSkipped(&result, servicePhases)

		if isSkipped {
			symbol = pterm.Gray(symbolSkipped)
			status = pterm.Gray("(skipped)")
			duration = "-"
		} else if result.Success {
			symbol = pterm.Green(symbolSuccess)
			status = ""
			duration = formatDuration(result.Duration)
		} else {
			symbol = pterm.Red(symbolFailure)
			status = pterm.Red("← FAILED")
			duration = formatDuration(result.Duration)
		}

		// Format the line content with fixed widths
		servicePhase := fmt.Sprintf("%s %s", result.Service, result.Phase)

		// Calculate status length for padding
		statusLen := 0
		if status != "" {
			if isSkipped {
				statusLen = len("(skipped)")
			} else {
				statusLen = len("← FAILED")
			}
		}

		// Calculate actual visible length: "  " + symbol + " " + servicePhase(24) + " " + duration(8) + "  " + status
		visibleLen := 2 + 1 + 1 + 24 + 1 + 8 + 2 + statusLen
		padding := contentWidth - visibleLen

		if padding < 0 {
			padding = 0
		}

		// Truncate servicePhase if needed
		if len(servicePhase) > 24 {
			servicePhase = servicePhase[:24]
		}

		// Print the row
		fmt.Printf("%s  %s %-24s %8s  %s%s%s\n",
			boxVertical,
			symbol,
			servicePhase,
			duration,
			status,
			strings.Repeat(" ", padding),
			boxVertical)
	}

	// Print bottom border.
	fmt.Println(boxBottomLeft + strings.Repeat(boxHorizontal, boxWidth-2) + boxBottomRight)

	// Print summary line.
	totalDuration := formatDuration(report.Duration)

	if report.Success {
		fmt.Printf("\n%s %s in %s\n", pterm.Green(symbolSuccess),
			pterm.Green(fmt.Sprintf("All %d steps completed successfully", report.TotalCount)),
			totalDuration)
	} else {
		fmt.Printf("\n%s %s\n", pterm.Red(symbolFailure),
			pterm.Red(fmt.Sprintf("%d/%d steps failed in %s",
				report.FailedCount, report.TotalCount, totalDuration)))
	}
}

// DisplayFailureSummary shows details of failed steps including command, exit code,
// and first 20 lines of stderr (or stdout if stderr empty).
func DisplayFailureSummary(report *diagnostic.RebuildReport) {
	if report == nil {
		return
	}

	failed := report.Failed()
	if len(failed) == 0 {
		return
	}

	Blank()
	Header("Failure Details")
	Blank()

	for i, result := range failed {
		// Print failure header.
		fmt.Printf("%s %s\n",
			pterm.Red(fmt.Sprintf("[%d]", i+1)),
			pterm.Bold.Sprint(fmt.Sprintf("%s - %s", result.Service, result.Phase)))

		// Print command.
		fmt.Printf("    %s %s\n", pterm.Gray("Command:"), result.Command)

		// Print exit code.
		fmt.Printf("    %s %d\n", pterm.Gray("Exit Code:"), result.ExitCode)

		// Print working directory if available.
		if result.WorkDir != "" {
			fmt.Printf("    %s %s\n", pterm.Gray("Directory:"), result.WorkDir)
		}

		// Get output to display.
		output := result.Stderr
		outputLabel := "Stderr"

		if strings.TrimSpace(output) == "" {
			output = result.Stdout
			outputLabel = "Stdout"
		}

		// Truncate and display output.
		if strings.TrimSpace(output) != "" {
			lines := strings.Split(output, "\n")
			truncated := false

			if len(lines) > maxFailureOutputLines {
				lines = lines[:maxFailureOutputLines]
				truncated = true
			}

			fmt.Printf("\n    %s\n", pterm.Gray(outputLabel+":"))

			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					fmt.Printf("    %s\n", pterm.Gray(line))
				}
			}

			if truncated {
				fmt.Printf("    %s\n", pterm.Yellow("... (output truncated)"))
			}
		}

		// Add spacing between failures.
		if i < len(failed)-1 {
			Blank()
			fmt.Println(strings.Repeat("─", 40))
		}

		Blank()
	}
}

// DisplayAIDiagnosis shows Claude's analysis in a formatted display.
// Format:
// AI Diagnosis
//
// Root Cause:
// [diagnosis.RootCause]
//
// Explanation:
// [diagnosis.Explanation]
//
// Affected Files:
// - file1.go
// - file2.proto
//
// Suggestions:
// 1. [suggestion 1]
// 2. [suggestion 2]
//
// Fix Commands:
// $ command1
// $ command2.
func DisplayAIDiagnosis(diagnosis *diagnostic.AIDiagnosis) {
	if diagnosis == nil {
		return
	}

	Blank()
	fmt.Printf("%s %s\n", pterm.Cyan("AI Diagnosis"), "")
	Blank()

	// Display root cause.
	if diagnosis.RootCause != "" {
		fmt.Printf("%s\n", pterm.Bold.Sprint("Root Cause:"))
		fmt.Printf("%s\n", diagnosis.RootCause)
		Blank()
	}

	// Display explanation.
	if diagnosis.Explanation != "" {
		fmt.Printf("%s\n", pterm.Bold.Sprint("Explanation:"))
		fmt.Printf("%s\n", diagnosis.Explanation)
		Blank()
	}

	// Display affected files.
	if len(diagnosis.AffectedFiles) > 0 {
		fmt.Printf("%s\n", pterm.Bold.Sprint("Affected Files:"))

		for _, file := range diagnosis.AffectedFiles {
			fmt.Printf("  %s %s\n", pterm.Gray("-"), file)
		}

		Blank()
	}

	// Display suggestions.
	if len(diagnosis.Suggestions) > 0 {
		fmt.Printf("%s\n", pterm.Bold.Sprint("Suggestions:"))

		for i, suggestion := range diagnosis.Suggestions {
			fmt.Printf("  %s %s\n", pterm.Cyan(fmt.Sprintf("%d.", i+1)), suggestion)
		}

		Blank()
	}

	// Display fix commands.
	if len(diagnosis.FixCommands) > 0 {
		fmt.Printf("%s\n", pterm.Bold.Sprint("Fix Commands:"))

		for _, cmd := range diagnosis.FixCommands {
			fmt.Printf("  %s %s\n", pterm.Yellow("$"), cmd)
		}

		Blank()
	}
}

// DisplayPatternDiagnosis shows pattern-matched hints for a build result.
// Format:
// Pattern Match: [pattern name]
//
// Hint:
// [diag.Hint]
//
// Suggestion:
// [diag.Suggestion].
func DisplayPatternDiagnosis(result *diagnostic.BuildResult, diag *diagnostic.Diagnosis) {
	if diag == nil || !diag.Matched {
		return
	}

	Blank()
	fmt.Printf("%s %s: %s\n",
		pterm.Yellow("Pattern Match"),
		pterm.Gray(""),
		pterm.Bold.Sprint(diag.PatternName))

	// Show confidence level.
	if diag.Confidence != "" {
		var confidenceColor func(a ...any) string

		switch diag.Confidence {
		case "high":
			confidenceColor = pterm.Green
		case "medium":
			confidenceColor = pterm.Yellow
		default:
			confidenceColor = pterm.Gray
		}

		fmt.Printf("%s %s\n", pterm.Gray("Confidence:"), confidenceColor(diag.Confidence))
	}

	Blank()

	// Display hint.
	if diag.Hint != "" {
		fmt.Printf("%s\n", pterm.Bold.Sprint("Hint:"))
		fmt.Printf("%s\n", diag.Hint)
		Blank()
	}

	// Display suggestion.
	if diag.Suggestion != "" {
		fmt.Printf("%s\n", pterm.Bold.Sprint("Suggestion:"))

		// Format multiline suggestions nicely.
		lines := strings.Split(diag.Suggestion, "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				fmt.Printf("%s\n", line)
			} else {
				Blank()
			}
		}

		Blank()
	}
}

// DisplayRawError shows the raw error when no pattern matches.
// Format:
// [service] [phase] failed
//
// Command: [command]
// Exit Code: [exit code]
//
// Output:
// [first 30 lines of stderr/stdout].
func DisplayRawError(result *diagnostic.BuildResult) {
	if result == nil {
		return
	}

	Blank()
	fmt.Printf("%s %s %s %s\n",
		pterm.Red(symbolFailure),
		pterm.Bold.Sprint(result.Service),
		pterm.Bold.Sprint(string(result.Phase)),
		pterm.Red("failed"))

	Blank()

	// Print command.
	fmt.Printf("%s %s\n", pterm.Gray("Command:"), result.Command)

	// Print exit code.
	fmt.Printf("%s %d\n", pterm.Gray("Exit Code:"), result.ExitCode)

	// Print working directory if available.
	if result.WorkDir != "" {
		fmt.Printf("%s %s\n", pterm.Gray("Directory:"), result.WorkDir)
	}

	// Get output to display.
	output := result.Stderr
	outputLabel := "Stderr"

	if strings.TrimSpace(output) == "" {
		output = result.Stdout
		outputLabel = "Stdout"
	}

	// Display output.
	if strings.TrimSpace(output) != "" {
		Blank()
		fmt.Printf("%s\n", pterm.Bold.Sprint("Output:"))

		lines := strings.Split(output, "\n")
		truncated := false

		if len(lines) > maxRawErrorLines {
			lines = lines[:maxRawErrorLines]
			truncated = true
		}

		for _, line := range lines {
			fmt.Printf("%s\n", pterm.Gray(line))
		}

		if truncated {
			fmt.Printf("\n%s\n", pterm.Yellow(fmt.Sprintf("... (%s truncated to %d lines)",
				outputLabel, maxRawErrorLines)))
		}
	}

	Blank()
}

// DisplayReportSummary shows a single report in list view format.
// Format:
// [2024-01-15 10:30:45] ID: abc123 | 2/7 failed | Duration: 5.2s
//
//	Failed: cbt-api (proto-gen), lab-backend (build)
func DisplayReportSummary(report *diagnostic.RebuildReport, serviceFilter string, verbose bool) {
	if report == nil {
		return
	}

	// Apply service filter if provided.
	filteredResults := report.Results
	if serviceFilter != "" {
		filteredResults = make([]diagnostic.BuildResult, 0, len(report.Results))

		for _, result := range report.Results {
			if result.Service == serviceFilter {
				filteredResults = append(filteredResults, result)
			}
		}

		if len(filteredResults) == 0 {
			return // No results for this service.
		}
	}

	// Count failures in filtered results.
	failedCount := 0
	totalCount := len(filteredResults)
	failedItems := make([]string, 0, len(filteredResults))

	for _, result := range filteredResults {
		if !result.Success {
			failedCount++

			failedItems = append(failedItems,
				fmt.Sprintf("%s (%s)", result.Service, result.Phase))
		}
	}

	// Format timestamp.
	timestamp := report.StartTime.Format("2006-01-02 15:04:05")

	// Format short ID (first 8 chars).
	shortID := report.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	// Format duration.
	duration := formatDuration(report.Duration)

	// Build the summary line.
	var statusText string

	if failedCount == 0 {
		statusText = pterm.Green(fmt.Sprintf("%d/%d passed", totalCount, totalCount))
	} else {
		statusText = pterm.Red(fmt.Sprintf("%d/%d failed", failedCount, totalCount))
	}

	fmt.Printf("[%s] %s: %s | %s | %s: %s\n",
		pterm.Gray(timestamp),
		pterm.Gray("ID"),
		shortID,
		statusText,
		pterm.Gray("Duration"),
		duration)

	// Show failed items on second line.
	if len(failedItems) > 0 {
		fmt.Printf("  %s %s\n",
			pterm.Red("Failed:"),
			strings.Join(failedItems, ", "))
	}

	// Verbose mode: show all results.
	if verbose {
		for _, result := range filteredResults {
			var symbol string

			if result.Success {
				symbol = pterm.Green(symbolSuccess)
			} else {
				symbol = pterm.Red(symbolFailure)
			}

			fmt.Printf("  %s %s %s %s\n",
				symbol,
				result.Service,
				result.Phase,
				formatDuration(result.Duration))
		}
	}
}

// formatDuration formats a duration in a human-readable format.
// Examples: "0.3s", "2.1s", "1m 5s", "2h 30m".
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	// For durations less than 1 second.
	if d < time.Second {
		ms := d.Milliseconds()
		if ms > 0 {
			return fmt.Sprintf("%dms", ms)
		}

		return "0s"
	}

	// For durations less than 1 minute.
	if d < time.Minute {
		secs := d.Seconds()

		return fmt.Sprintf("%.1fs", secs)
	}

	// For durations less than 1 hour.
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60

		if secs == 0 {
			return fmt.Sprintf("%dm", mins)
		}

		return fmt.Sprintf("%dm %ds", mins, secs)
	}

	// For durations of 1 hour or more.
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60

	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}

	return fmt.Sprintf("%dh %dm", hours, mins)
}

// isResultSkipped determines if a result should be marked as skipped.
// A result is skipped if it has zero duration and a previous phase of the same service failed.
func isResultSkipped(result *diagnostic.BuildResult, servicePhases map[string][]diagnostic.BuildResult) bool {
	// If the result has a duration or ran (even if briefly), it's not skipped.
	if result.Duration > 0 || result.ExitCode != 0 || result.Stderr != "" || result.Stdout != "" {
		return false
	}

	// Check if any earlier phase of this service failed.
	phases := servicePhases[result.Service]

	for _, phase := range phases {
		// If we've reached the current result, stop checking.
		if phase.Phase == result.Phase && phase.StartTime.Equal(result.StartTime) {
			break
		}

		// If an earlier phase failed, this result is skipped.
		if !phase.Success {
			return true
		}
	}

	return false
}
