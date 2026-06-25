package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RepoVersion is a fast local version snapshot for a git repository.
type RepoVersion struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}

// Snapshot reads branch, commit SHA, and dirty state without fetching.
func Snapshot(ctx context.Context, repoPath string) (RepoVersion, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return RepoVersion{Path: repoPath}, fmt.Errorf("failed to resolve repo path: %w", err)
	}

	version := RepoVersion{Path: absPath}

	if _, statErr := os.Stat(absPath); statErr != nil {
		return version, fmt.Errorf("failed to inspect repo path %s: %w", absPath, statErr)
	}

	if _, workTreeErr := gitOutput(ctx, absPath, "rev-parse", "--is-inside-work-tree"); workTreeErr != nil {
		return version, fmt.Errorf("not a git worktree at %s: %w", absPath, workTreeErr)
	}

	branch, err := gitOutput(ctx, absPath, "branch", "--show-current")
	if err != nil {
		return version, fmt.Errorf("failed to read branch: %w", err)
	}

	if branch == "" {
		branch, err = gitOutput(ctx, absPath, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return version, fmt.Errorf("failed to read detached branch state: %w", err)
		}
	}

	commit, err := gitOutput(ctx, absPath, "rev-parse", "HEAD")
	if err != nil {
		return version, fmt.Errorf("failed to read commit: %w", err)
	}

	status, err := gitOutput(ctx, absPath, "status", "--porcelain")
	if err != nil {
		return version, fmt.Errorf("failed to read dirty state: %w", err)
	}

	version.Branch = branch
	version.Commit = commit
	version.Dirty = status != ""

	return version, nil
}

func gitOutput(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}
