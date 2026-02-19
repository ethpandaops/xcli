package release

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// runStatus represents the JSON response from gh run view.
type runStatus struct {
	Status     string    `json:"status"`     // "queued", "in_progress", "completed"
	Conclusion string    `json:"conclusion"` // "success", "failure", "cancelled", null
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	HTMLURL    string    `json:"url"`
	HeadSha    string    `json:"headSha"` // Git commit SHA
}

// WatchRun polls a workflow run until completion or timeout.
func (s *service) WatchRun(
	ctx context.Context,
	repo string,
	runID string,
	opts WatchOptions,
) (*WatchResult, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Minute
	}

	if opts.PollInterval == 0 {
		opts.PollInterval = 30 * time.Second
	}

	result := &WatchResult{
		RunID: runID,
	}

	startTime := time.Now()
	deadline := startTime.Add(opts.Timeout)
	ticker := time.NewTicker(opts.PollInterval)

	defer ticker.Stop()

	// Do an immediate check first
	status, err := s.getRunStatus(ctx, repo, runID)
	if err != nil {
		result.Error = fmt.Errorf("failed to get initial run status: %w", err)

		return result, result.Error
	}

	result.WorkflowURL = status.HTMLURL

	// Check if already completed
	if status.Status == "completed" {
		return s.finalizeWatchResult(ctx, result, status, repo, startTime)
	}

	// Poll until completion or timeout
	for {
		select {
		case <-ctx.Done():
			result.Status = "cancelled"
			result.Error = ctx.Err()

			return result, result.Error

		case <-ticker.C:
			// Check timeout
			if time.Now().After(deadline) {
				result.Status = StatusTimedOut
				result.Duration = time.Since(startTime)
				result.Error = fmt.Errorf("timed out after %v waiting for workflow to complete", opts.Timeout)

				return result, result.Error
			}

			// Get current status
			status, err = s.getRunStatus(ctx, repo, runID)
			if err != nil {
				s.log.WithError(err).Warn("failed to get run status, will retry")

				continue
			}

			s.log.WithFields(map[string]any{
				"runID":    runID,
				"status":   status.Status,
				"elapsed":  time.Since(startTime).Round(time.Second),
				"deadline": time.Until(deadline).Round(time.Second),
			}).Debug("polling workflow run")

			if status.Status == "completed" {
				return s.finalizeWatchResult(ctx, result, status, repo, startTime)
			}
		}
	}
}

// finalizeWatchResult populates the final result after completion.
func (s *service) finalizeWatchResult(
	ctx context.Context,
	result *WatchResult,
	status *runStatus,
	repo string,
	startTime time.Time,
) (*WatchResult, error) {
	result.Status = status.Status
	result.Conclusion = status.Conclusion
	result.Duration = time.Since(startTime)
	result.WorkflowURL = status.HTMLURL
	result.HeadSha = status.HeadSha

	// Get artifacts info
	artifacts, err := s.getArtifacts(ctx, repo, result.RunID)
	if err != nil {
		s.log.WithError(err).Debug("failed to get artifacts")
	} else {
		result.Artifacts = artifacts
	}

	if result.Conclusion != StatusSuccess {
		result.Error = fmt.Errorf("workflow completed with conclusion: %s", result.Conclusion)
	}

	return result, nil
}

// getRunStatus fetches current status of a workflow run.
func (s *service) getRunStatus(ctx context.Context, repo, runID string) (*runStatus, error) {
	output, err := s.runGH(ctx, "run", "view",
		runID,
		"--repo", repo,
		"--json", "status,conclusion,createdAt,updatedAt,url,headSha")
	if err != nil {
		return nil, fmt.Errorf("failed to get run status: %w", err)
	}

	var status runStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		return nil, fmt.Errorf("failed to parse run status: %w", err)
	}

	return &status, nil
}

// getArtifacts returns human-readable artifact descriptions for a completed run.
func (s *service) getArtifacts(ctx context.Context, repo, runID string) ([]string, error) {
	// For now, just return a generic message about where to find artifacts
	// A more sophisticated implementation could parse the workflow summary
	artifacts := []string{
		fmt.Sprintf("View artifacts: https://github.com/%s/actions/runs/%s", repo, runID),
	}

	return artifacts, nil
}

// MultiWatchResult aggregates watch results for multiple workflow runs.
type MultiWatchResult struct {
	Results   map[string]*WatchResult // Keyed by project name
	StartTime time.Time
	EndTime   time.Time
}

// Duration returns total watch duration.
func (m *MultiWatchResult) Duration() time.Duration {
	return m.EndTime.Sub(m.StartTime)
}

// AllSucceeded returns true if all watched builds succeeded.
func (m *MultiWatchResult) AllSucceeded() bool {
	for _, r := range m.Results {
		if r.Conclusion != StatusSuccess {
			return false
		}
	}

	return len(m.Results) > 0
}

// Get returns the watch result for a specific project.
func (m *MultiWatchResult) Get(project string) *WatchResult {
	if m.Results == nil {
		return nil
	}

	return m.Results[project]
}

// WatchItem represents a single item to watch.
type WatchItem struct {
	Project string // Project name for identification
	Repo    string // Full repo (e.g., "ethpandaops/cbt")
	RunID   string // GitHub Actions run ID
}

// WatchMultiple watches multiple workflow runs concurrently.
// Returns results as each completes, respects context cancellation.
// Uses a single timeout for the entire operation.
func (s *service) WatchMultiple(
	ctx context.Context,
	items []WatchItem,
	opts WatchOptions,
	onUpdate func(project string, status string),
) (*MultiWatchResult, error) {
	if len(items) == 0 {
		return &MultiWatchResult{
			Results:   make(map[string]*WatchResult),
			StartTime: time.Now(),
			EndTime:   time.Now(),
		}, nil
	}

	result := &MultiWatchResult{
		Results:   make(map[string]*WatchResult, len(items)),
		StartTime: time.Now(),
	}

	// Set default timeout if not specified
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Minute
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// Use errgroup for concurrent watching
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	for _, item := range items {
		g.Go(func() error {
			watchResult, err := s.WatchRun(gctx, item.Repo, item.RunID, opts)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.Results[item.Project] = &WatchResult{
					RunID:  item.RunID,
					Status: "error",
					Error:  err,
				}

				if onUpdate != nil {
					onUpdate(item.Project, "error")
				}
			} else {
				result.Results[item.Project] = watchResult

				if onUpdate != nil {
					onUpdate(item.Project, watchResult.Conclusion)
				}
			}

			return nil // Don't propagate errors - we want all watches to complete
		})
	}

	if err := g.Wait(); err != nil {
		s.log.WithError(err).Warn("error waiting for watch group")
	}

	result.EndTime = time.Now()

	return result, nil
}
