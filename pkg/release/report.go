package release

import (
	"fmt"
	"strings"
	"time"
)

// ReleaseReport provides formatted output for multi-project releases.
type ReleaseReport struct {
	entries   []ReportEntry
	startTime time.Time
	endTime   time.Time
}

// ReportEntry represents a single project's release status.
type ReportEntry struct {
	Project   string
	Version   string        // New version or "-" for workflow dispatch
	Duration  time.Duration // Build duration (from watch)
	Status    string        // "success", "failed", "skipped", "pending", "released"
	Error     string        // Error message if failed
	URL       string        // Workflow run URL
	DependsOn []string      // Dependencies (for display)
	Skipped   bool          // True if skipped due to dependency failure
}

// NewReleaseReport creates a new release report.
func NewReleaseReport() *ReleaseReport {
	return &ReleaseReport{
		entries:   make([]ReportEntry, 0),
		startTime: time.Now(),
	}
}

// AddRelease records a release attempt.
func (r *ReleaseReport) AddRelease(result *ReleaseResult, dependsOn []string) {
	version := result.Version
	if version == "" {
		version = "-"
	}

	var status string

	var errMsg string

	if result.Success {
		status = StatusReleased
	} else {
		status = StatusFailed

		if result.Error != nil {
			errMsg = result.Error.Error()
		}
	}

	r.entries = append(r.entries, ReportEntry{
		Project:   result.Project,
		Version:   version,
		Status:    status,
		Error:     errMsg,
		URL:       result.WorkflowURL,
		DependsOn: dependsOn,
	})
}

// AddSkipped records a skipped release (due to dependency failure).
func (r *ReleaseReport) AddSkipped(project, version, reason string, dependsOn []string) {
	r.entries = append(r.entries, ReportEntry{
		Project:   project,
		Version:   version,
		Status:    StatusSkipped,
		Error:     reason,
		DependsOn: dependsOn,
		Skipped:   true,
	})
}

// UpdateWithWatchResult updates an entry with watch result.
func (r *ReleaseReport) UpdateWithWatchResult(project string, watchResult *WatchResult) {
	for i := range r.entries {
		if r.entries[i].Project == project {
			r.entries[i].Duration = watchResult.Duration

			if watchResult.Conclusion == StatusSuccess {
				r.entries[i].Status = StatusSuccess
			} else if watchResult.Status == StatusTimedOut {
				r.entries[i].Status = StatusTimedOut
				r.entries[i].Error = "build timed out"
			} else {
				r.entries[i].Status = StatusFailed

				if watchResult.Error != nil {
					r.entries[i].Error = watchResult.Error.Error()
				} else {
					r.entries[i].Error = fmt.Sprintf("build %s", watchResult.Conclusion)
				}
			}

			break
		}
	}
}

// UpdateFromMultiWatch updates entries with multi-watch results.
func (r *ReleaseReport) UpdateFromMultiWatch(watchResult *MultiWatchResult) {
	for project, wr := range watchResult.Results {
		r.UpdateWithWatchResult(project, wr)
	}
}

// GetEntry returns the entry for a project.
func (r *ReleaseReport) GetEntry(project string) *ReportEntry {
	for i := range r.entries {
		if r.entries[i].Project == project {
			return &r.entries[i]
		}
	}

	return nil
}

// Finalize marks the report as complete.
func (r *ReleaseReport) Finalize() {
	r.endTime = time.Now()
}

// TotalDuration returns total elapsed time.
func (r *ReleaseReport) TotalDuration() time.Duration {
	if r.endTime.IsZero() {
		return time.Since(r.startTime)
	}

	return r.endTime.Sub(r.startTime)
}

// Succeeded returns count of successful releases.
func (r *ReleaseReport) Succeeded() int {
	count := 0

	for _, e := range r.entries {
		if e.Status == StatusSuccess {
			count++
		}
	}

	return count
}

// Failed returns count of failed releases.
func (r *ReleaseReport) Failed() int {
	count := 0

	for _, e := range r.entries {
		if e.Status == StatusFailed || e.Status == StatusTimedOut {
			count++
		}
	}

	return count
}

// Skipped returns count of skipped releases.
func (r *ReleaseReport) Skipped() int {
	count := 0

	for _, e := range r.entries {
		if e.Skipped {
			count++
		}
	}

	return count
}

// Entries returns the report entries for display.
func (r *ReleaseReport) Entries() []ReportEntry {
	return r.entries
}

// FormatSummaryLine returns a formatted summary.
func (r *ReleaseReport) FormatSummaryLine() string {
	parts := make([]string, 0, 3)

	if s := r.Succeeded(); s > 0 {
		parts = append(parts, fmt.Sprintf("%d succeeded", s))
	}

	if f := r.Failed(); f > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", f))
	}

	if s := r.Skipped(); s > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", s))
	}

	summary := strings.Join(parts, ", ")
	duration := FormatDuration(r.TotalDuration())

	return fmt.Sprintf("%s | Duration: %s", summary, duration)
}

// FormatDuration formats a duration as "Xm Ys" or "Xs".
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}

	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60

	return fmt.Sprintf("%dm %ds", minutes, seconds)
}
