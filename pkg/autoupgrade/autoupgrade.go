package autoupgrade

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
)

// Service manages automatic upgrades of xcli.
type Service interface {
	// CheckAndUpgrade checks for updates and upgrades if needed.
	CheckAndUpgrade(ctx context.Context) error
}

type service struct {
	log           logrus.FieldLogger
	repoPath      string
	checkInterval time.Duration
	currentCommit string
}

// NewService creates a new auto-upgrade service.
func NewService(log logrus.FieldLogger, repoPath, currentCommit string) Service {
	return &service{
		log:           log.WithField("package", "autoupgrade"),
		repoPath:      repoPath,
		checkInterval: 1 * time.Hour,
		currentCommit: currentCommit,
	}
}

// CheckAndUpgrade performs the upgrade check and execution.
func (s *service) CheckAndUpgrade(ctx context.Context) error {
	// Load global config
	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		s.log.WithError(err).Debug("Failed to load global config")

		return nil // Don't fail the CLI if config load fails
	}

	// Check if we should perform upgrade check based on last check time
	if !globalCfg.LastUpgradeCheck.IsZero() && time.Since(globalCfg.LastUpgradeCheck) < s.checkInterval {
		s.log.Debug("Skipping upgrade check (too soon)")

		return nil
	}

	// Update last check time
	globalCfg.LastUpgradeCheck = time.Now()
	globalCfg.LastUpgradeCommit = s.currentCommit

	if saveErr := config.SaveGlobalConfig(globalCfg); saveErr != nil {
		s.log.WithError(saveErr).Debug("Failed to save global config")
	}

	// Check if we're in a git repository
	if !s.isGitRepo() {
		s.log.Debug("Not a git repository, skipping upgrade check")

		return nil
	}

	// Fetch latest changes
	if fetchErr := s.gitFetch(ctx); fetchErr != nil {
		s.log.WithError(fetchErr).Debug("Failed to fetch updates")

		return nil // Don't fail the CLI if upgrade check fails
	}

	// Check if we're behind upstream
	behind, err := s.isBehindUpstream(ctx)
	if err != nil {
		s.log.WithError(err).Debug("Failed to check upstream status")

		return nil
	}

	if !behind {
		s.log.Debug("Already up to date")

		return nil
	}

	// Perform upgrade
	if upgradeErr := s.performUpgrade(ctx); upgradeErr != nil {
		s.log.WithError(upgradeErr).Warn("Auto-upgrade failed")

		return nil // Don't fail the CLI if upgrade fails
	}

	fmt.Printf("%s %s\n", ui.SuccessSymbol, ui.SuccessStyle.Sprint("xcli upgraded to latest version (will take effect on next run)"))

	return nil
}

// isGitRepo checks if the current directory is a git repository.
func (s *service) isGitRepo() bool {
	gitDir := filepath.Join(s.repoPath, ".git")
	info, err := os.Stat(gitDir)

	return err == nil && info.IsDir()
}

// gitFetch fetches the latest changes from origin.
func (s *service) gitFetch(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "fetch", "origin", "--quiet")
	cmd.Dir = s.repoPath

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch failed: %w: %s", err, string(output))
	}

	return nil
}

// isBehindUpstream checks if the current branch is behind origin.
func (s *service) isBehindUpstream(ctx context.Context) (bool, error) {
	// Get current branch
	branchCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = s.repoPath

	branchOutput, err := branchCmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get current branch: %w", err)
	}

	branch := strings.TrimSpace(string(branchOutput))

	// Get local commit
	localCmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	localCmd.Dir = s.repoPath

	localOutput, err := localCmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get local commit: %w", err)
	}

	localCommit := strings.TrimSpace(string(localOutput))

	// Get remote commit
	// The branch name comes from git rev-parse output, which is safe
	remoteRef := "origin/" + branch
	remoteCmd := exec.CommandContext(ctx, "git", "rev-parse", remoteRef)
	remoteCmd.Dir = s.repoPath

	remoteOutput, err := remoteCmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get remote commit: %w", err)
	}

	remoteCommit := strings.TrimSpace(string(remoteOutput))

	return localCommit != remoteCommit, nil
}

// performUpgrade executes git pull and make install.
func (s *service) performUpgrade(ctx context.Context) error {
	s.log.Info("Upgrading xcli to latest version...")

	// Git pull
	pullCmd := exec.CommandContext(ctx, "git", "pull", "--quiet")
	pullCmd.Dir = s.repoPath

	if output, err := pullCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull failed: %w: %s", err, string(output))
	}

	// Make install
	installCmd := exec.CommandContext(ctx, "make", "install")
	installCmd.Dir = s.repoPath
	installCmd.Env = os.Environ()

	if output, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("make install failed: %w: %s", err, string(output))
	}

	return nil
}
