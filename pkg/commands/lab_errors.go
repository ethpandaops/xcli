package commands

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethpandaops/xcli/pkg/diagnostic"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/pterm/pterm"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabErrorsCommand creates the lab errors command.
func NewLabErrorsCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var limit int
	var service string
	var verbose bool

	cmd := &cobra.Command{
		Use:   "errors",
		Short: "View recent build errors",
		Long: `Display history of recent build failures.

Examples:
  xcli lab errors              # Show last 5 errors
  xcli lab errors --limit 10   # Show last 10 errors
  xcli lab errors --service cbt-api  # Filter by service
  xcli lab errors -v           # Show verbose details`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLabErrors(log, configPath, limit, service, verbose)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 5, "Number of reports to show")
	cmd.Flags().StringVar(&service, "service", "", "Filter by service name")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show full error details")

	return cmd
}

func runLabErrors(
	log logrus.FieldLogger,
	configPath string,
	limit int,
	service string,
	verbose bool,
) error {
	// Determine the errors directory relative to config path
	configDir := filepath.Dir(configPath)
	if configDir == "" || configDir == "." {
		configDir = "."
	}

	errorsDir := filepath.Join(configDir, ".xcli", "errors")

	// Create store
	store := diagnostic.NewStore(log, errorsDir)

	// Load reports
	reports, err := store.List(limit)
	if err != nil {
		return fmt.Errorf("failed to load error reports: %w", err)
	}

	// Check if no reports exist
	if len(reports) == 0 {
		ui.Info("No build errors in history")

		return nil
	}

	// Display header
	ui.Header("Build Error History")
	fmt.Println()

	// Display each report
	for _, report := range reports {
		displayReportSummary(report, service, verbose)
	}

	return nil
}

// displayReportSummary shows a single report in list view.
// Format: [2024-01-15 10:30:45] ID: abc123 | 2/7 failed | Duration: 5.2s
//
//	Failed: cbt-api (proto-gen), lab-backend (build)
func displayReportSummary(
	report *diagnostic.RebuildReport,
	serviceFilter string,
	verbose bool,
) {
	// Get failed results, optionally filtered by service
	failed := report.Failed()

	// Apply service filter if specified
	if serviceFilter != "" {
		filtered := make([]diagnostic.BuildResult, 0, len(failed))

		for _, result := range failed {
			if result.Service == serviceFilter {
				filtered = append(filtered, result)
			}
		}

		failed = filtered
	}

	// Skip this report if service filter is applied and no matching failures
	if serviceFilter != "" && len(failed) == 0 {
		return
	}

	// Format timestamp
	timestamp := report.StartTime.Format("2006-01-02 15:04:05")

	// Format duration
	duration := formatDuration(report.Duration)

	// Build summary line
	statusColor := pterm.FgRed
	if report.Success {
		statusColor = pterm.FgGreen
	}

	statusStyle := pterm.NewStyle(statusColor)

	// Display main summary line
	fmt.Printf("[%s] ID: %s | %s | Duration: %s\n",
		timestamp,
		report.ID[:8], // Short ID
		statusStyle.Sprintf("%d/%d failed", report.FailedCount, report.TotalCount),
		duration,
	)

	// Display failed services
	if len(failed) > 0 {
		var failedServices []string

		for _, result := range failed {
			failedServices = append(failedServices,
				fmt.Sprintf("%s (%s)", result.Service, result.Phase))
		}

		fmt.Printf("  Failed: %s\n", pterm.Red(strings.Join(failedServices, ", ")))
	}

	// Display verbose details if requested
	if verbose && len(failed) > 0 {
		fmt.Println()

		for _, result := range failed {
			displayBuildResultDetails(result)
		}
	}

	fmt.Println()
}

// displayBuildResultDetails shows detailed information for a single build result.
func displayBuildResultDetails(result diagnostic.BuildResult) {
	// Service and phase header
	boldStyle := pterm.NewStyle(pterm.Bold)
	fmt.Printf("  %s [%s]:\n", boldStyle.Sprint(result.Service), result.Phase)

	// Command executed
	if result.Command != "" {
		fmt.Printf("    Command: %s\n", pterm.Gray(result.Command))
	}

	// Working directory
	if result.WorkDir != "" {
		fmt.Printf("    WorkDir: %s\n", pterm.Gray(result.WorkDir))
	}

	// Exit code
	if result.ExitCode != 0 {
		fmt.Printf("    Exit Code: %d\n", result.ExitCode)
	}

	// Error message
	if result.ErrorMsg != "" {
		fmt.Printf("    Error: %s\n", pterm.Red(result.ErrorMsg))
	}

	// Show stderr (truncated if too long)
	if result.Stderr != "" {
		stderr := truncateOutput(result.Stderr, 30)
		fmt.Printf("    Stderr:\n")

		for _, line := range strings.Split(stderr, "\n") {
			if line != "" {
				fmt.Printf("      %s\n", line)
			}
		}
	}

	// Show stdout if no stderr (truncated if too long)
	if result.Stderr == "" && result.Stdout != "" {
		stdout := truncateOutput(result.Stdout, 30)
		fmt.Printf("    Stdout:\n")

		for _, line := range strings.Split(stdout, "\n") {
			if line != "" {
				fmt.Printf("      %s\n", line)
			}
		}
	}

	fmt.Println()
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}

	return fmt.Sprintf("%.1fs", d.Seconds())
}

// truncateOutput truncates output to a maximum number of lines.
func truncateOutput(output string, maxLines int) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}

	truncated := strings.Join(lines[:maxLines], "\n")

	return truncated + fmt.Sprintf("\n... (%d more lines)", len(lines)-maxLines)
}
