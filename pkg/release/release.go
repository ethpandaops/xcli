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
	Error       error         // Error if watch itself failed
}

// BumpType represents the type of version bump.
type BumpType string

const (
	BumpPatch BumpType = "patch"
	BumpMinor BumpType = "minor"
	BumpMajor BumpType = "major"
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
