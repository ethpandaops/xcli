// Package release provides GitHub release operations for lab stack components.
// It uses the GitHub CLI (gh) for all GitHub interactions.
package release

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// ProjectInfo contains information about a releasable project.
type ProjectInfo struct {
	Name           string // e.g., "cbt"
	Repo           string // e.g., "ethpandaops/cbt"
	CurrentVersion string // e.g., "v0.5.2" or "N/A" for non-semver
	IsSemver       bool   // true for tag-triggered projects
	Description    string // Human-readable description
}

// ReleaseResult contains the outcome of a release operation.
type ReleaseResult struct {
	Project     string // Project that was released
	Version     string // New version (empty for non-semver)
	WorkflowURL string // URL to the triggered workflow run
	RunID       string // GitHub Actions run ID (for watching)
	Repo        string // Full repo name (e.g., "ethpandaops/cbt")
	Success     bool
	Error       error
}

// WatchResult contains the final status after watching a workflow run.
type WatchResult struct {
	RunID       string        // GitHub Actions run ID
	Status      string        // "completed", "failed", "cancelled", "timed_out"
	Conclusion  string        // "success", "failure", "cancelled", etc.
	Duration    time.Duration // How long the build took
	WorkflowURL string        // URL to the workflow run
	Artifacts   []string      // List of artifact descriptions
	HeadSha     string        // Git commit SHA that was built
	Error       error         // Error if watch itself failed
}

// BumpType represents the type of version bump.
type BumpType string

const (
	BumpPatch BumpType = "patch"
	BumpMinor BumpType = "minor"
	BumpMajor BumpType = "major"
)

// Status constants for workflow runs and releases.
const (
	StatusSuccess  = "success"
	StatusFailed   = "failed"
	StatusTimedOut = "timed_out"
	StatusSkipped  = "skipped"
	StatusPending  = "pending"
	StatusReleased = "released"
)

// WatchOptions configures the watch behavior.
type WatchOptions struct {
	Timeout      time.Duration // Max time to wait (default: 30m)
	PollInterval time.Duration // How often to poll (default: 30s)
}

// DefaultWatchOptions returns sensible defaults for watching.
func DefaultWatchOptions() WatchOptions {
	return WatchOptions{
		Timeout:      30 * time.Minute,
		PollInterval: 30 * time.Second,
	}
}

// Service provides release operations.
type Service interface {
	// CheckPrerequisites verifies gh CLI is installed and authenticated
	CheckPrerequisites(ctx context.Context) error

	// GetProjectInfo fetches current version info for all releasable projects
	GetProjectInfo(ctx context.Context) ([]ProjectInfo, error)

	// ReleaseSemver creates a new semver release by pushing a tag
	ReleaseSemver(ctx context.Context, project string, bumpType BumpType) (*ReleaseResult, error)

	// ReleaseWorkflow triggers a workflow dispatch for non-semver projects
	ReleaseWorkflow(ctx context.Context, project string) (*ReleaseResult, error)

	// WatchRun polls a workflow run until completion or timeout
	// Returns WatchResult with final status, duration, and artifacts
	WatchRun(ctx context.Context, repo string, runID string, opts WatchOptions) (*WatchResult, error)

	// WatchMultiple watches multiple workflow runs concurrently
	// Returns results as each completes, respects context cancellation
	WatchMultiple(ctx context.Context, items []WatchItem, opts WatchOptions, onUpdate func(project string, status string)) (*MultiWatchResult, error)
}

// NewService creates a new release service.
func NewService(log logrus.FieldLogger) Service {
	return &service{
		log: log.WithField("package", "release"),
	}
}

type service struct {
	log logrus.FieldLogger
}

// ProjectReleaseConfig holds release configuration for a single project.
type ProjectReleaseConfig struct {
	Project    string       // Project name (e.g., "cbt")
	BumpType   BumpType     // For semver projects (ignored for workflow dispatch)
	NewVersion string       // Calculated new version (for display)
	Info       *ProjectInfo // Project info (version, semver status)
}

// MultiReleaseResult aggregates results from releasing multiple projects.
type MultiReleaseResult struct {
	Results   []ReleaseResult // Individual release results
	StartTime time.Time       // When the multi-release started
	EndTime   time.Time       // When the multi-release finished
}

// Succeeded returns the count of successful releases.
func (m *MultiReleaseResult) Succeeded() int {
	count := 0

	for _, r := range m.Results {
		if r.Success {
			count++
		}
	}

	return count
}

// Failed returns the count of failed releases.
func (m *MultiReleaseResult) Failed() int {
	count := 0

	for _, r := range m.Results {
		if !r.Success {
			count++
		}
	}

	return count
}

// Duration returns the total duration of the multi-release.
func (m *MultiReleaseResult) Duration() time.Duration {
	return m.EndTime.Sub(m.StartTime)
}

// AllSucceeded returns true if all releases succeeded.
func (m *MultiReleaseResult) AllSucceeded() bool {
	for _, r := range m.Results {
		if !r.Success {
			return false
		}
	}

	return len(m.Results) > 0
}
