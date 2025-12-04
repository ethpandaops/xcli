package release

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/ethpandaops/xcli/pkg/constants"
)

// ghRelease represents the JSON response from gh release list.
type ghRelease struct {
	TagName      string `json:"tagName"`
	IsPrerelease bool   `json:"isPrerelease"`
	IsDraft      bool   `json:"isDraft"`
}

// ghRunInfo represents the JSON response from gh run list.
type ghRunInfo struct {
	DatabaseID int64  `json:"databaseId"`
	URL        string `json:"url"`
}

// CheckPrerequisites verifies gh CLI is installed and authenticated.
func (s *service) CheckPrerequisites(ctx context.Context) error {
	// Check if gh is installed
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found: please install it from https://cli.github.com")
	}

	// Check if gh is authenticated
	output, err := s.runGH(ctx, "auth", "status")
	if err != nil {
		return fmt.Errorf("gh not authenticated: run 'gh auth login' first: %w", err)
	}

	s.log.WithField("output", output).Debug("gh auth status")

	return nil
}

// GetProjectInfo fetches current versions for all projects.
func (s *service) GetProjectInfo(ctx context.Context) ([]ProjectInfo, error) {
	projects := make([]ProjectInfo, 0, len(constants.ReleasableProjects))

	for _, project := range constants.ReleasableProjects {
		info, err := s.getProjectInfo(ctx, project)
		if err != nil {
			s.log.WithError(err).WithField("project", project).Warn("failed to get project info")
			// Continue with partial info rather than failing completely
			info = &ProjectInfo{
				Name:           project,
				Repo:           fmt.Sprintf("%s/%s", constants.GitHubOrg, project),
				CurrentVersion: "unknown",
				IsSemver:       slices.Contains(constants.SemverProjects, project),
				Description:    "failed to fetch version",
			}
		}

		projects = append(projects, *info)
	}

	return projects, nil
}

// getProjectInfo fetches info for a single project.
func (s *service) getProjectInfo(ctx context.Context, project string) (*ProjectInfo, error) {
	repo := fmt.Sprintf("%s/%s", constants.GitHubOrg, project)
	isSemver := slices.Contains(constants.SemverProjects, project)

	info := &ProjectInfo{
		Name:     project,
		Repo:     repo,
		IsSemver: isSemver,
	}

	if isSemver {
		// Fetch latest release
		output, err := s.runGH(ctx, "release", "list",
			"--repo", repo,
			"--limit", "10",
			"--json", "tagName,isPrerelease,isDraft")
		if err != nil {
			return nil, fmt.Errorf("failed to list releases: %w", err)
		}

		var releases []ghRelease
		if err := json.Unmarshal([]byte(output), &releases); err != nil {
			return nil, fmt.Errorf("failed to parse releases: %w", err)
		}

		// Find first non-prerelease, non-draft release
		info.CurrentVersion = "no releases"

		for _, rel := range releases {
			if !rel.IsPrerelease && !rel.IsDraft {
				info.CurrentVersion = rel.TagName
				info.Description = fmt.Sprintf("current: %s", rel.TagName)

				break
			}
		}

		if info.Description == "" {
			info.Description = "no stable releases"
		}
	} else {
		// xatu-cbt - no semver, just workflow dispatch
		info.CurrentVersion = "N/A"
		info.Description = "triggers docker build on master"
	}

	return info, nil
}

// ReleaseSemver creates a new semver release by creating a GitHub release.
func (s *service) ReleaseSemver(
	ctx context.Context,
	project string,
	bumpType BumpType,
) (*ReleaseResult, error) {
	repo := fmt.Sprintf("%s/%s", constants.GitHubOrg, project)

	result := &ReleaseResult{
		Project: project,
		Repo:    repo,
	}

	// Get current version
	info, err := s.getProjectInfo(ctx, project)
	if err != nil {
		result.Error = fmt.Errorf("failed to get current version: %w", err)

		return result, result.Error
	}

	if info.CurrentVersion == "no releases" {
		result.Error = fmt.Errorf("no existing releases found for %s; create first release manually", project)

		return result, result.Error
	}

	// Parse and bump version
	currentVersion, err := ParseVersion(info.CurrentVersion)
	if err != nil {
		result.Error = fmt.Errorf("failed to parse current version %s: %w", info.CurrentVersion, err)

		return result, result.Error
	}

	newVersion := currentVersion.Bump(bumpType)
	result.Version = newVersion.String()

	s.log.WithFields(map[string]any{
		"project":        project,
		"currentVersion": info.CurrentVersion,
		"newVersion":     result.Version,
		"bumpType":       bumpType,
	}).Info("creating release")

	// Create the release using gh
	_, err = s.runGH(ctx, "release", "create",
		result.Version,
		"--repo", repo,
		"--generate-notes",
		"--title", result.Version)
	if err != nil {
		result.Error = fmt.Errorf("failed to create release: %w", err)

		return result, result.Error
	}

	// Wait a moment for the workflow to be triggered
	time.Sleep(2 * time.Second)

	// Get the triggered workflow run
	runInfo, err := s.getLatestWorkflowRun(ctx, repo)
	if err != nil {
		s.log.WithError(err).Warn("failed to get workflow run info")
		// Don't fail - the release was created successfully
	} else {
		result.WorkflowURL = runInfo.URL
		result.RunID = fmt.Sprintf("%d", runInfo.DatabaseID)
	}

	result.Success = true

	return result, nil
}

// ReleaseWorkflow triggers a workflow dispatch for non-semver projects.
func (s *service) ReleaseWorkflow(ctx context.Context, project string) (*ReleaseResult, error) {
	repo := fmt.Sprintf("%s/%s", constants.GitHubOrg, project)

	result := &ReleaseResult{
		Project: project,
		Repo:    repo,
	}

	// Determine workflow file
	var workflowFile string

	switch project {
	case constants.ProjectXatuCBT:
		workflowFile = constants.WorkflowXatuCBTDocker
	default:
		result.Error = fmt.Errorf("unknown workflow for project: %s", project)

		return result, result.Error
	}

	s.log.WithFields(map[string]any{
		"project":  project,
		"workflow": workflowFile,
	}).Info("triggering workflow dispatch")

	// Trigger the workflow
	_, err := s.runGH(ctx, "workflow", "run",
		workflowFile,
		"--repo", repo)
	if err != nil {
		result.Error = fmt.Errorf("failed to trigger workflow: %w", err)

		return result, result.Error
	}

	// Wait a moment for the workflow run to be created
	time.Sleep(3 * time.Second)

	// Get the triggered workflow run
	runInfo, err := s.getWorkflowRun(ctx, repo, workflowFile)
	if err != nil {
		s.log.WithError(err).Warn("failed to get workflow run info")
		// Don't fail - the workflow was triggered
	} else {
		result.WorkflowURL = runInfo.URL
		result.RunID = fmt.Sprintf("%d", runInfo.DatabaseID)
	}

	result.Success = true

	return result, nil
}

// getLatestWorkflowRun gets the most recent workflow run for a repo.
func (s *service) getLatestWorkflowRun(ctx context.Context, repo string) (*ghRunInfo, error) {
	output, err := s.runGH(ctx, "run", "list",
		"--repo", repo,
		"--limit", "1",
		"--json", "databaseId,url")
	if err != nil {
		return nil, fmt.Errorf("failed to list runs: %w", err)
	}

	var runs []ghRunInfo
	if err := json.Unmarshal([]byte(output), &runs); err != nil {
		return nil, fmt.Errorf("failed to parse runs: %w", err)
	}

	if len(runs) == 0 {
		return nil, fmt.Errorf("no workflow runs found")
	}

	return &runs[0], nil
}

// getWorkflowRun gets the most recent run for a specific workflow.
func (s *service) getWorkflowRun(ctx context.Context, repo, workflow string) (*ghRunInfo, error) {
	output, err := s.runGH(ctx, "run", "list",
		"--repo", repo,
		"--workflow", workflow,
		"--limit", "1",
		"--json", "databaseId,url")
	if err != nil {
		return nil, fmt.Errorf("failed to list runs: %w", err)
	}

	var runs []ghRunInfo
	if err := json.Unmarshal([]byte(output), &runs); err != nil {
		return nil, fmt.Errorf("failed to parse runs: %w", err)
	}

	if len(runs) == 0 {
		return nil, fmt.Errorf("no workflow runs found")
	}

	return &runs[0], nil
}

// runGH executes a gh command and returns output.
func (s *service) runGH(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	s.log.WithField("args", args).Debug("running gh command")

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}

		return "", fmt.Errorf("gh %s: %s", strings.Join(args, " "), errMsg)
	}

	return strings.TrimSpace(stdout.String()), nil
}
