// Package prerequisites manages repository setup requirements including
// file copying, directory checks, and command execution.
package prerequisites

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
)

// NewChecker creates a new prerequisites checker with known repo definitions.
func NewChecker(log logrus.FieldLogger) Checker {
	return &checker{
		log:  log.WithField("component", "prerequisites"),
		defs: buildKnownRepoPrerequisites(),
	}
}

// Check validates if prerequisites are met for a repo.
func (c *checker) Check(ctx context.Context, repoPath string, repoName string) error {
	prereqs, exists := c.defs[repoName]
	if !exists {
		// No prerequisites defined for this repo
		return nil
	}

	for _, prereq := range prereqs.Prerequisites {
		if err := c.checkPrerequisite(ctx, repoPath, prereq); err != nil {
			return fmt.Errorf("prerequisite not met: %w", err)
		}
	}

	return nil
}

// Run executes prerequisites for a repo.
func (c *checker) Run(ctx context.Context, repoPath string, repoName string) error {
	prereqs, exists := c.defs[repoName]
	if !exists {
		// No prerequisites defined for this repo
		return nil
	}

	if len(prereqs.Prerequisites) == 0 {
		return nil
	}

	c.log.WithField("repo", repoName).Info("running prerequisites")

	for _, prereq := range prereqs.Prerequisites {
		// Check if we should skip this prerequisite
		if prereq.SkipIfExists != "" {
			skipPath := filepath.Join(repoPath, prereq.SkipIfExists)
			if _, err := os.Stat(skipPath); err == nil {
				c.log.WithFields(logrus.Fields{
					"repo":        repoName,
					"description": prereq.Description,
				}).Debug("skipping prerequisite (target already exists)")

				continue
			}
		}

		c.log.WithFields(logrus.Fields{
			"repo":        repoName,
			"description": prereq.Description,
		}).Info("executing prerequisite")

		var err error

		switch prereq.Type {
		case PrerequisiteTypeFileCopy:
			err = c.executeFileCopy(ctx, repoPath, prereq)
		case PrerequisiteTypeCommand:
			err = c.executeCommand(ctx, repoPath, prereq)
		case PrerequisiteTypeDirectoryCheck:
			err = c.executeDirectoryCheck(ctx, repoPath, prereq)
		default:
			err = fmt.Errorf("unknown prerequisite type: %s", prereq.Type)
		}

		if err != nil {
			return fmt.Errorf("failed to execute prerequisite '%s': %w", prereq.Description, err)
		}
	}

	c.log.WithField("repo", repoName).Info("prerequisites complete")

	return nil
}

// CheckAndRun checks prerequisites and runs them if needed.
func (c *checker) CheckAndRun(ctx context.Context, repoPath string, repoName string) error {
	if err := c.Check(ctx, repoPath, repoName); err != nil {
		// Prerequisites not met, try to run them
		return c.Run(ctx, repoPath, repoName)
	}

	// Prerequisites already met
	return nil
}

// checkPrerequisite validates if a single prerequisite is met.
func (c *checker) checkPrerequisite(ctx context.Context, repoPath string, prereq Prerequisite) error {
	switch prereq.Type {
	case PrerequisiteTypeFileCopy:
		// Check if destination file exists
		destPath := filepath.Join(repoPath, prereq.DestinationPath)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			return fmt.Errorf("destination file does not exist: %s", prereq.DestinationPath)
		}

	case PrerequisiteTypeCommand:
		// For commands, check if the expected artifact exists (using SkipIfExists)
		if prereq.SkipIfExists != "" {
			artifactPath := filepath.Join(repoPath, prereq.SkipIfExists)
			if _, err := os.Stat(artifactPath); os.IsNotExist(err) {
				return fmt.Errorf("command artifact does not exist: %s", prereq.SkipIfExists)
			}
		}

	case PrerequisiteTypeDirectoryCheck:
		dirPath := filepath.Join(repoPath, prereq.DirectoryPath)
		_, err := os.Stat(dirPath)

		if prereq.ShouldExist && os.IsNotExist(err) {
			return fmt.Errorf("directory should exist but does not: %s", prereq.DirectoryPath)
		}

		if !prereq.ShouldExist && err == nil {
			return fmt.Errorf("directory should not exist but does: %s", prereq.DirectoryPath)
		}
	}

	return nil
}

// executeFileCopy copies a file from source to destination.
func (c *checker) executeFileCopy(ctx context.Context, repoPath string, prereq Prerequisite) error {
	srcPath := filepath.Join(repoPath, prereq.SourcePath)
	destPath := filepath.Join(repoPath, prereq.DestinationPath)

	// Check if source exists
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("source file does not exist: %s", prereq.SourcePath)
	}

	// Read source file
	content, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read source file %s: %w", prereq.SourcePath, err)
	}

	// Write to destination
	if err := os.WriteFile(destPath, content, 0600); err != nil {
		return fmt.Errorf("failed to write destination file %s: %w", prereq.DestinationPath, err)
	}

	c.log.WithFields(logrus.Fields{
		"source": prereq.SourcePath,
		"dest":   prereq.DestinationPath,
	}).Debug("file copied successfully")

	return nil
}

// executeCommand runs a shell command in the repo directory.
func (c *checker) executeCommand(ctx context.Context, repoPath string, prereq Prerequisite) error {
	workDir := repoPath
	if prereq.WorkingDir != "" && prereq.WorkingDir != "." {
		workDir = filepath.Join(repoPath, prereq.WorkingDir)
	}

	// Start spinner with prerequisite description
	spinner := ui.NewSilentSpinner(prereq.Description)

	start := time.Now()

	//nolint:gosec // Command comes from hardcoded prerequisite definitions, not user input
	cmd := exec.CommandContext(ctx, prereq.Command, prereq.Args...)
	cmd.Dir = workDir
	cmd.Env = os.Environ() // Inherit environment variables

	// Capture output for error messages
	output, err := cmd.CombinedOutput()

	duration := time.Since(start)

	if err != nil {
		spinner.Fail(prereq.Description)

		return fmt.Errorf(
			"command failed: %w\nCommand: %s %v\nOutput: %s",
			err,
			prereq.Command,
			prereq.Args,
			string(output),
		)
	}

	// Stop silently - parent lab_init spinner shows overall prereq success
	_ = spinner.Stop()

	c.log.WithFields(logrus.Fields{
		"command":  prereq.Command,
		"args":     prereq.Args,
		"duration": duration,
	}).Debug("command executed successfully")

	return nil
}

// executeDirectoryCheck validates directory existence.
func (c *checker) executeDirectoryCheck(ctx context.Context, repoPath string, prereq Prerequisite) error {
	dirPath := filepath.Join(repoPath, prereq.DirectoryPath)

	info, err := os.Stat(dirPath)

	if prereq.ShouldExist {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory should exist but does not: %s", prereq.DirectoryPath)
		}

		if err != nil {
			return fmt.Errorf("failed to check directory %s: %w", prereq.DirectoryPath, err)
		}

		if !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory: %s", prereq.DirectoryPath)
		}
	} else {
		if err == nil {
			return fmt.Errorf("directory should not exist but does: %s", prereq.DirectoryPath)
		}

		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to check directory %s: %w", prereq.DirectoryPath, err)
		}
	}

	return nil
}
