// Package git provides utilities for checking git repository status
// and ensuring repositories are up to date with their remotes.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

// RepoStatus represents the status of a git repository.
type RepoStatus struct {
	Path             string
	Name             string
	IsUpToDate       bool
	BehindBy         int
	AheadBy          int
	CurrentBranch    string
	HasUncommitted   bool   // True if there are uncommitted changes
	UncommittedCount int    // Number of uncommitted files
	CommitsSinceTag  int    // Commits since last tag (release)
	LatestTag        string // Most recent tag
	Error            error
}

// Checker checks git repository status.
type Checker struct {
	log logrus.FieldLogger
}

// NewChecker creates a new git checker.
func NewChecker(log logrus.FieldLogger) *Checker {
	return &Checker{
		log: log.WithField("component", "git-checker"),
	}
}

// CheckRepository checks if a repository is up to date with its remote.
// It performs a git fetch to get the latest remote refs, then compares
// the local and remote branches.
func (c *Checker) CheckRepository(ctx context.Context, repoPath string, repoName string) RepoStatus {
	status := RepoStatus{
		Path:       repoPath,
		Name:       repoName,
		IsUpToDate: true,
	}

	// Check if directory exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		status.IsUpToDate = false
		status.Error = fmt.Errorf("repository not found")

		return status
	}

	// Check for uncommitted changes
	uncommitted, err := c.getUncommittedCount(ctx, repoPath)
	if err != nil {
		c.log.WithError(err).WithField("repo", repoName).Debug("failed to check uncommitted changes")
	} else {
		status.UncommittedCount = uncommitted
		status.HasUncommitted = uncommitted > 0
	}

	// Get latest tag and commits since
	latestTag, err := c.getLatestTag(ctx, repoPath)
	if err != nil {
		c.log.WithError(err).WithField("repo", repoName).Debug("failed to get latest tag (repo may have no tags)")
	} else {
		status.LatestTag = latestTag

		commitsSince, tagErr := c.getCommitsSinceTag(ctx, repoPath, latestTag)
		if tagErr != nil {
			c.log.WithError(tagErr).WithField("repo", repoName).Debug("failed to count commits since tag")
		} else {
			status.CommitsSinceTag = commitsSince
		}
	}

	// Get current branch
	branch, err := c.getCurrentBranch(ctx, repoPath)
	if err != nil {
		status.IsUpToDate = false
		status.Error = fmt.Errorf("failed to get current branch: %w", err)

		return status
	}

	status.CurrentBranch = branch

	// Fetch latest from remote (quietly)
	if fetchErr := c.fetchRemote(ctx, repoPath); fetchErr != nil {
		c.log.WithError(fetchErr).WithField("repo", repoName).Debug("failed to fetch remote (continuing without remote check)")
		// Don't fail the check if fetch fails - repo might not have remote configured
		return status
	}

	// Get remote tracking branch
	remoteBranch, err := c.getRemoteTrackingBranch(ctx, repoPath, branch)
	if err != nil {
		c.log.WithError(err).WithField("repo", repoName).Debug("no remote tracking branch (continuing)")
		// Don't fail if there's no remote tracking branch
		return status
	}

	// Check how many commits behind/ahead we are
	behind, ahead, err := c.getCommitDifference(ctx, repoPath, branch, remoteBranch)
	if err != nil {
		status.IsUpToDate = false
		status.Error = fmt.Errorf("failed to check commit difference: %w", err)

		return status
	}

	status.BehindBy = behind
	status.AheadBy = ahead
	status.IsUpToDate = (behind == 0 && ahead == 0)

	return status
}

// CheckRepositories checks multiple repositories for their git status.
// Returns a slice of RepoStatus for each repository.
func (c *Checker) CheckRepositories(ctx context.Context, repos map[string]string) []RepoStatus {
	statuses := make([]RepoStatus, 0, len(repos))

	for name, path := range repos {
		status := c.CheckRepository(ctx, path, name)
		statuses = append(statuses, status)
	}

	return statuses
}

// getCurrentBranch returns the current branch name for a repository.
func (c *Checker) getCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// fetchRemote fetches the latest refs from the remote repository.
func (c *Checker) fetchRemote(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "git", "fetch", "--quiet")
	cmd.Dir = repoPath

	// Capture stderr to suppress output
	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git fetch failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// getRemoteTrackingBranch returns the remote tracking branch for the given local branch.
func (c *Checker) getRemoteTrackingBranch(ctx context.Context, repoPath string, branch string) (string, error) {
	//nolint:gosec // branch is from git output, not user input
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", fmt.Sprintf("%s@{upstream}", branch))
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// getCommitDifference returns how many commits behind and ahead the local branch is
// compared to the remote tracking branch.
func (c *Checker) getCommitDifference(ctx context.Context, repoPath string, localBranch string, remoteBranch string) (behind int, ahead int, err error) {
	// Check how many commits behind
	//nolint:gosec // localBranch and remoteBranch are from git output, not user input
	behindCmd := exec.CommandContext(ctx, "git", "rev-list", "--count", fmt.Sprintf("%s..%s", localBranch, remoteBranch))
	behindCmd.Dir = repoPath

	behindOutput, err := behindCmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to check commits behind: %w", err)
	}

	_, err = fmt.Sscanf(strings.TrimSpace(string(behindOutput)), "%d", &behind)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse behind count: %w", err)
	}

	// Check how many commits ahead
	//nolint:gosec // localBranch and remoteBranch are from git output, not user input
	aheadCmd := exec.CommandContext(ctx, "git", "rev-list", "--count", fmt.Sprintf("%s..%s", remoteBranch, localBranch))
	aheadCmd.Dir = repoPath

	aheadOutput, err := aheadCmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to check commits ahead: %w", err)
	}

	_, err = fmt.Sscanf(strings.TrimSpace(string(aheadOutput)), "%d", &ahead)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse ahead count: %w", err)
	}

	return behind, ahead, nil
}

// getUncommittedCount returns the number of uncommitted files (staged + unstaged + untracked).
func (c *Checker) getUncommittedCount(ctx context.Context, repoPath string) (int, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	if len(output) == 0 {
		return 0, nil
	}

	// Count non-empty lines
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	return len(lines), nil
}

// getLatestTag returns the most recent tag in the repository.
func (c *Checker) getLatestTag(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "describe", "--tags", "--abbrev=0")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// getCommitsSinceTag returns the number of commits since the given tag.
func (c *Checker) getCommitsSinceTag(ctx context.Context, repoPath string, tag string) (int, error) {
	//nolint:gosec // tag is from git output, not user input
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--count", fmt.Sprintf("%s..HEAD", tag))
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	var count int

	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count)
	if err != nil {
		return 0, err
	}

	return count, nil
}
