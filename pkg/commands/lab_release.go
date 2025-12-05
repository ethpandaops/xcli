package commands

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/release"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabReleaseCommand creates the lab release command.
func NewLabReleaseCommand(log logrus.FieldLogger, _ string) *cobra.Command {
	var (
		bumpFlag    string
		yesFlag     bool
		noWatchFlag bool
		timeoutFlag time.Duration
	)

	cmd := &cobra.Command{
		Use:   "release [project]",
		Short: "Trigger a release build for a lab stack component",
		Long: `Trigger a release build for a lab stack component.

This command helps you create releases for lab stack projects by:
  - Showing current versions of all releasable projects
  - Guiding you through version selection (for semver projects)
  - Triggering the appropriate build workflow
  - Watching the build until completion (default behavior)

Supported projects:
  cbt          - ClickHouse transformation tool (semver, tag-triggered)
  cbt-api      - REST API for CBT (semver, tag-triggered)
  lab-backend  - Lab API gateway (semver, tag-triggered)
  xatu-cbt     - CBT models and migrations (workflow dispatch, no semver)

Examples:
  xcli lab release                    # Interactive project selection, watch build
  xcli lab release cbt                # Release cbt with version prompt
  xcli lab release cbt --bump patch   # Release cbt patch version
  xcli lab release xatu-cbt           # Trigger xatu-cbt docker build
  xcli lab release cbt --no-watch     # Trigger release without watching
  xcli lab release cbt --timeout 1h   # Watch with custom timeout

Note: Requires GitHub CLI (gh) to be installed and authenticated.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRelease(cmd, log, args, bumpFlag, yesFlag, !noWatchFlag, timeoutFlag)
		},
	}

	cmd.Flags().StringVarP(&bumpFlag, "bump", "b", "", "Version bump type: patch, minor, major")
	cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Skip confirmation prompts")
	cmd.Flags().BoolVar(&noWatchFlag, "no-watch", false, "Don't watch the build (just trigger and exit)")
	cmd.Flags().DurationVar(&timeoutFlag, "timeout", 30*time.Minute, "Timeout for watching build")

	return cmd
}

// runRelease orchestrates the release flow.
func runRelease(
	cmd *cobra.Command,
	log logrus.FieldLogger,
	args []string,
	bumpFlag string,
	skipConfirm, watch bool,
	timeout time.Duration,
) error {
	ctx := cmd.Context()
	svc := release.NewService(log)

	// Step 1: Check prerequisites
	ui.Header("Checking prerequisites")

	if err := svc.CheckPrerequisites(ctx); err != nil {
		ui.Error("Prerequisites check failed")

		return err
	}

	ui.Success("GitHub CLI authenticated")
	ui.Blank()

	// Step 2: Get project info
	ui.Header("Fetching project versions")

	projects, err := svc.GetProjectInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get project info: %w", err)
	}

	ui.Blank()

	// Step 3: Select project
	var selectedProject string

	if len(args) > 0 {
		selectedProject = args[0]
		// Validate project name
		if !slices.Contains(constants.ReleasableProjects, selectedProject) {
			return fmt.Errorf("unknown project: %s (valid: %v)", selectedProject, constants.ReleasableProjects)
		}
	} else {
		// Interactive selection
		options := make([]ui.SelectOption, 0, len(projects))

		for _, p := range projects {
			options = append(options, ui.SelectOption{
				Label:       p.Name,
				Description: p.Description,
				Value:       p.Name,
			})
		}

		selected, selectErr := ui.Select("Select project to release", options)
		if selectErr != nil {
			return fmt.Errorf("project selection cancelled: %w", selectErr)
		}

		selectedProject = selected
	}

	// Find selected project info
	var projectInfo *release.ProjectInfo

	for i := range projects {
		if projects[i].Name == selectedProject {
			projectInfo = &projects[i]

			break
		}
	}

	if projectInfo == nil {
		return fmt.Errorf("project not found: %s", selectedProject)
	}

	ui.Info(fmt.Sprintf("Selected: %s (%s)", projectInfo.Name, projectInfo.Description))
	ui.Blank()

	// Step 4: Handle release based on project type
	var result *release.ReleaseResult

	if projectInfo.IsSemver {
		result, err = handleSemverRelease(ctx, svc, projectInfo, bumpFlag, skipConfirm)
	} else {
		result, err = handleWorkflowRelease(ctx, svc, projectInfo, skipConfirm)
	}

	if err != nil {
		return err
	}

	// Step 5: Display result
	ui.Blank()
	ui.Success(fmt.Sprintf("Release triggered for %s", result.Project))

	if result.Version != "" {
		ui.Info(fmt.Sprintf("Version: %s", result.Version))
	}

	if result.WorkflowURL != "" {
		ui.Info(fmt.Sprintf("Workflow: %s", result.WorkflowURL))
	}

	// Step 6: Watch build if requested
	if watch && result.RunID != "" {
		ui.Blank()
		ui.Header("Watching build progress")

		opts := release.WatchOptions{
			Timeout:      timeout,
			PollInterval: 30 * time.Second,
		}

		spinner := ui.NewSpinner(fmt.Sprintf("Building %s...", result.Project))

		watchResult, watchErr := svc.WatchRun(ctx, result.Repo, result.RunID, opts)
		if watchErr != nil {
			spinner.Fail("Build watch failed")
			ui.Warning(fmt.Sprintf("Failed to watch build: %v", watchErr))
			ui.Info(fmt.Sprintf("Check build status at: %s", result.WorkflowURL))

			return nil // Don't fail - the release was triggered
		}

		if watchResult.Conclusion == "success" {
			spinner.Success(fmt.Sprintf("Build completed in %s", watchResult.Duration.Round(time.Second)))

			for _, artifact := range watchResult.Artifacts {
				ui.Info(artifact)
			}
		} else {
			spinner.Fail(fmt.Sprintf("Build %s after %s", watchResult.Conclusion, watchResult.Duration.Round(time.Second)))
			ui.Error(fmt.Sprintf("Check logs at: %s", watchResult.WorkflowURL))
		}
	} else if !watch {
		ui.Blank()
		ui.Info("Skipping build watch (--no-watch)")
	}

	return nil
}

// handleSemverRelease handles releases for semver projects.
func handleSemverRelease(
	ctx context.Context,
	svc release.Service,
	project *release.ProjectInfo,
	bumpFlag string,
	skipConfirm bool,
) (*release.ReleaseResult, error) {
	// Determine bump type
	var bumpType release.BumpType

	if bumpFlag != "" {
		switch bumpFlag {
		case "patch":
			bumpType = release.BumpPatch
		case "minor":
			bumpType = release.BumpMinor
		case "major":
			bumpType = release.BumpMajor
		default:
			return nil, fmt.Errorf("invalid bump type: %s (valid: patch, minor, major)", bumpFlag)
		}
	} else {
		// Interactive bump selection
		options := []ui.SelectOption{
			{Label: "patch", Description: "Bug fixes (0.0.X)", Value: "patch"},
			{Label: "minor", Description: "New features (0.X.0)", Value: "minor"},
			{Label: "major", Description: "Breaking changes (X.0.0)", Value: "major"},
		}

		selected, err := ui.SelectWithDefault("Select version bump type", options, "patch")
		if err != nil {
			return nil, fmt.Errorf("bump selection cancelled: %w", err)
		}

		switch selected {
		case "patch":
			bumpType = release.BumpPatch
		case "minor":
			bumpType = release.BumpMinor
		case "major":
			bumpType = release.BumpMajor
		}
	}

	// Calculate new version for confirmation
	currentVersion, err := release.ParseVersion(project.CurrentVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse current version: %w", err)
	}

	newVersion := currentVersion.Bump(bumpType)

	// Confirm
	if !skipConfirm {
		ui.Blank()
		ui.Info(fmt.Sprintf("Current version: %s", project.CurrentVersion))
		ui.Info(fmt.Sprintf("New version: %s", newVersion.String()))
		ui.Blank()

		confirmed, confirmErr := ui.Confirm(fmt.Sprintf("Create release %s for %s?", newVersion.String(), project.Name))
		if confirmErr != nil || !confirmed {
			return nil, fmt.Errorf("release cancelled")
		}
	}

	return svc.ReleaseSemver(ctx, project.Name, bumpType)
}

// handleWorkflowRelease handles releases for workflow dispatch projects.
func handleWorkflowRelease(
	ctx context.Context,
	svc release.Service,
	project *release.ProjectInfo,
	skipConfirm bool,
) (*release.ReleaseResult, error) {
	// Confirm
	if !skipConfirm {
		ui.Info(fmt.Sprintf("This will trigger the docker build workflow for %s", project.Name))
		ui.Blank()

		confirmed, err := ui.Confirm(fmt.Sprintf("Trigger docker build for %s?", project.Name))
		if err != nil || !confirmed {
			return nil, fmt.Errorf("release cancelled")
		}
	}

	return svc.ReleaseWorkflow(ctx, project.Name)
}
