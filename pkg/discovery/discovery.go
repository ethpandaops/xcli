// Package discovery handles repository discovery and cloning
// for the lab stack components.
package discovery

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
)

// Discovery handles repository discovery.
type Discovery struct {
	log      logrus.FieldLogger
	basePath string
}

// NewDiscovery creates a new Discovery instance.
func NewDiscovery(log logrus.FieldLogger, basePath string) *Discovery {
	return &Discovery{
		log:      log.WithField("component", "discovery"),
		basePath: basePath,
	}
}

// DiscoverRepos attempts to find all required lab repositories.
// If a repository is missing, it will prompt the user to clone it.
func (d *Discovery) DiscoverRepos(ctx context.Context) (*config.LabReposConfig, error) {
	d.log.Info("discovering lab repositories")

	repos := &config.LabReposConfig{
		CBT:        filepath.Join(d.basePath, "cbt"),
		XatuCBT:    filepath.Join(d.basePath, "xatu-cbt"),
		CBTAPI:     filepath.Join(d.basePath, "cbt-api"),
		LabBackend: filepath.Join(d.basePath, "lab-backend"),
		Lab:        filepath.Join(d.basePath, "lab"),
	}

	// Map of repo names to their paths, GitHub repo names, and optional branches
	repoMap := map[string]struct {
		path     *string
		repoName string
		branch   string // Optional: branch to checkout after cloning
	}{
		"cbt":         {&repos.CBT, constants.RepoCBT, ""},
		"xatu-cbt":    {&repos.XatuCBT, constants.RepoXatuCBT, ""},
		"cbt-api":     {&repos.CBTAPI, constants.RepoCBTAPI, ""},
		"lab-backend": {&repos.LabBackend, constants.RepoLabBackend, ""},
		"lab":         {&repos.Lab, constants.RepoLab, "release/frontend"},
	}

	// Check each repository
	for name, info := range repoMap {
		// Validate repository with spinner
		var validationErr error

		err := ui.WithSpinner(fmt.Sprintf("Checking repository: %s", name), func() error {
			validationErr = d.validateRepo(name, *info.path)

			return nil // Don't fail spinner on validation error, we handle it below
		})
		if err != nil {
			return nil, err
		}

		if validationErr != nil {
			// Check if it's a "not found" error
			absPath, _ := filepath.Abs(*info.path)
			if _, statErr := os.Stat(absPath); os.IsNotExist(statErr) {
				// Repository doesn't exist - prompt to clone
				fmt.Printf("\n⚠ Repository '%s' not found at: %s\n", name, absPath)

				// Show branch info if applicable
				branchInfo := ""
				if info.branch != "" {
					branchInfo = fmt.Sprintf(" (branch: %s)", info.branch)
				}

				fmt.Printf("Would you like to clone it from GitHub?%s (Y/n): ", branchInfo)

				var response string

				_, _ = fmt.Scanln(&response)

				// Default to yes if empty or "y"/"Y"
				if response == "" || response == "y" || response == "Y" {
					// Clone the repository (with optional branch)
					if cloneErr := d.cloneRepo(ctx, info.repoName, absPath, info.branch); cloneErr != nil {
						return nil, fmt.Errorf("failed to clone %s: %w", name, cloneErr)
					}

					// Validate again after cloning
					if validateErr := d.validateRepo(name, *info.path); validateErr != nil {
						return nil, fmt.Errorf("cloned repository %s failed validation: %w", name, validateErr)
					}
				} else {
					return nil, fmt.Errorf("repository %s is required but was not cloned", name)
				}
			} else {
				// Some other validation error
				return nil, fmt.Errorf("failed to validate %s: %w", name, validationErr)
			}
		}

		d.log.WithFields(logrus.Fields{
			"repo": name,
			"path": *info.path,
		}).Info("found repository")
	}

	return repos, nil
}

// validateRepo checks if a repository exists and has expected structure.
func (d *Discovery) validateRepo(name, path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check if directory exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("repository not found at %s", absPath)
		}

		return fmt.Errorf("failed to stat directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	// Check for expected files based on repo type
	switch name {
	case "cbt", "xatu-cbt", "cbt-api", "lab-backend":
		// Go repositories - check for go.mod
		if !d.fileExists(filepath.Join(absPath, "go.mod")) {
			return fmt.Errorf("go.mod not found (not a Go project)")
		}
	case "lab":
		// Frontend repository - check for package.json
		if !d.fileExists(filepath.Join(absPath, "package.json")) {
			return fmt.Errorf("package.json not found (not a Node.js project)")
		}
	}

	// Additional validation for specific repos
	switch name {
	case "xatu-cbt":
		// Check for models directory
		if !d.dirExists(filepath.Join(absPath, "models")) {
			return fmt.Errorf("models directory not found")
		}
	case "lab":
		// Check for src directory
		if !d.dirExists(filepath.Join(absPath, "src")) {
			return fmt.Errorf("src directory not found")
		}
	}

	return nil
}

// fileExists checks if a file exists.
func (d *Discovery) fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

// dirExists checks if a directory exists.
func (d *Discovery) dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.IsDir()
}

// cloneRepo clones a repository from GitHub and optionally checks out a specific branch.
func (d *Discovery) cloneRepo(ctx context.Context, repoName, targetPath, branch string) error {
	// Get GitHub URL for the repository
	gitURL := constants.GetGitHubURL(repoName)

	logFields := logrus.Fields{
		"repo":   repoName,
		"url":    gitURL,
		"path":   targetPath,
		"branch": branch,
	}

	d.log.WithFields(logFields).Info("cloning repository")

	// Create spinner for clone operation
	spinner := ui.NewSpinner(fmt.Sprintf("Cloning %s", repoName))

	// Ensure parent directory exists
	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		spinner.Fail(fmt.Sprintf("Failed to clone %s", repoName))

		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Run git clone with optional branch
	var cmd *exec.Cmd
	if branch != "" {
		// Clone and checkout specific branch directly
		cmd = exec.CommandContext(ctx, "git", "clone", "-b", branch, gitURL, targetPath)
	} else {
		// Clone default branch
		cmd = exec.CommandContext(ctx, "git", "clone", gitURL, targetPath)
	}

	// In verbose mode, show git output; otherwise, suppress it
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		spinner.Fail(fmt.Sprintf("Failed to clone %s", repoName))

		return fmt.Errorf("git clone failed: %w", err)
	}

	spinner.Success(fmt.Sprintf("Cloned %s", repoName))

	d.log.WithField("repo", repoName).Info("repository cloned successfully")

	return nil
}
