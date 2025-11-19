package commands

import (
	"fmt"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/diagnostic"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabDiagnoseCommand creates the lab diagnose command.
func NewLabDiagnoseCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var useAI bool

	var reportID string

	cmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Diagnose the most recent build failure",
		Long: `Analyze the most recent build failure and provide diagnosis.

By default, uses pattern matching for instant results.
Use --ai flag to get AI-powered analysis from Claude Code.

Examples:
  xcli lab diagnose           # Diagnose latest failure with patterns
  xcli lab diagnose --ai      # Use Claude Code for AI analysis
  xcli lab diagnose --id xxx  # Diagnose specific report by ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create store for diagnostic reports
			store := diagnostic.NewStore(log, filepath.Join(".xcli", "errors"))

			// Load report (latest or by ID)
			var report *diagnostic.RebuildReport

			var err error

			if reportID != "" {
				report, err = store.Load(reportID)
			} else {
				report, err = store.Latest()
			}

			if err != nil {
				return fmt.Errorf("no diagnostic reports found: %w", err)
			}

			// Check for failures
			if !report.HasFailures() {
				ui.Success("No failures in the selected report")
				ui.Info(fmt.Sprintf("Report ID: %s | %d steps completed successfully",
					report.ID, report.TotalCount))

				return nil
			}

			// Display failure summary first
			ui.DisplayFailureSummary(report)

			// Try AI diagnosis if requested
			if useAI {
				client, clientErr := diagnostic.NewClaudeClient(log)
				if clientErr != nil {
					ui.Warning("Claude Code not available: " + clientErr.Error())
					ui.Info("Falling back to pattern matching...")

					useAI = false
				} else {
					spinner := ui.NewSpinner("Analyzing with Claude Code...")

					diagnosis, diagErr := client.Diagnose(cmd.Context(), report)
					if diagErr != nil {
						spinner.Fail("AI diagnosis failed")
						ui.Warning(diagErr.Error())
						ui.Info("Falling back to pattern matching...")

						useAI = false
					} else {
						spinner.Success("Analysis complete")
						ui.DisplayAIDiagnosis(diagnosis)

						return nil
					}
				}
			}

			// Pattern matching fallback
			matcher := diagnostic.NewPatternMatcher()

			for _, result := range report.Failed() {
				diag := matcher.Match(&result)
				if diag != nil && diag.Matched {
					ui.DisplayPatternDiagnosis(&result, diag)
				} else {
					ui.DisplayRawError(&result)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&useAI, "ai", false, "Use Claude Code for AI-powered diagnosis")
	cmd.Flags().StringVar(&reportID, "id", "", "Diagnose specific report by ID")

	return cmd
}
