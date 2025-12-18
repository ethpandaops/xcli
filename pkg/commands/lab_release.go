package commands

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/git"
	"github.com/ethpandaops/xcli/pkg/release"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/pterm/pterm"
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
		stackFlag   bool
		noDepsFlag  bool
	)

	cmd := &cobra.Command{
		Use:   "release [project...]",
		Short: "Trigger a release build for lab stack component(s)",
		Long: `Trigger a release build for one or more lab stack components.

This command helps you create releases for lab stack projects by:
  - Showing current versions of all releasable projects
  - Respecting project dependencies (cbt must build before xatu-cbt)
  - Guiding you through version selection (for semver projects)
  - Watching builds until completion (default behavior)

Dependencies:
  xatu-cbt depends on cbt - when both are released, cbt builds first

Supported projects:
  cbt          - ClickHouse transformation tool (semver, tag-triggered)
  cbt-api      - REST API for CBT (semver, tag-triggered)
  lab-backend  - Lab API gateway (semver, tag-triggered)
  xatu-cbt     - CBT models and migrations (workflow dispatch, depends on cbt)

Single project:
  xcli lab release cbt                    # Release cbt with version prompt
  xcli lab release cbt --bump patch       # Release cbt patch version
  xcli lab release xatu-cbt               # Prompts about cbt dependency

Multiple projects:
  xcli lab release cbt xatu-cbt           # Release in dependency order
  xcli lab release --stack                # Release entire stack
  xcli lab release --stack --bump patch   # All semver projects get patch bump
  xcli lab release                        # Interactive multi-select

Options:
  xcli lab release cbt xatu-cbt --no-watch  # Trigger without watching
  xcli lab release --stack --timeout 1h     # Watch with custom timeout
  xcli lab release xatu-cbt --no-deps       # Skip dependency prompts

Note: Requires GitHub CLI (gh) to be installed and authenticated.`,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeReleasableProjects(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRelease(cmd.Context(), log, args, stackFlag, bumpFlag,
				yesFlag, !noWatchFlag, noDepsFlag, timeoutFlag)
		},
	}

	cmd.Flags().StringVarP(&bumpFlag, "bump", "b", "", "Version bump type: patch, minor, major")
	cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Skip confirmation prompts")
	cmd.Flags().BoolVar(&noWatchFlag, "no-watch", false, "Don't watch the build (just trigger and exit)")
	cmd.Flags().DurationVar(&timeoutFlag, "timeout", 30*time.Minute, "Timeout for watching build")
	cmd.Flags().BoolVar(&stackFlag, "stack", false, "Release entire stack with dependency ordering")
	cmd.Flags().BoolVar(&noDepsFlag, "no-deps", false, "Skip dependency checks and prompts")

	return cmd
}

// runRelease orchestrates the release flow.
func runRelease(
	ctx context.Context,
	log logrus.FieldLogger,
	args []string,
	releaseStack bool,
	bumpFlag string,
	skipConfirm, watch, skipDeps bool,
	timeout time.Duration,
) error {
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
	spinner := ui.NewSpinner("Fetching project versions...")

	projects, err := svc.GetProjectInfo(ctx)
	if err != nil {
		spinner.Fail("Failed to fetch project versions")

		return fmt.Errorf("failed to get project info: %w", err)
	}

	spinner.Success("Project versions fetched")

	// Display current versions
	ui.Blank()

	for _, p := range projects {
		if p.IsSemver {
			fmt.Printf("  • %s: %s\n", p.Name, pterm.Cyan(p.CurrentVersion))
		} else {
			// For workflow dispatch projects, show commit hash and description
			fmt.Printf("  • %s: %s (%s)\n", p.Name, pterm.Cyan(p.CurrentVersion), pterm.Gray(p.Description))
		}
	}

	ui.Blank()

	// Step 3: Determine selected projects
	var selectedProjects []string

	if releaseStack {
		selectedProjects = constants.ReleasableProjects
	} else if len(args) > 0 {
		selectedProjects = args
		// Validate each project
		for _, project := range selectedProjects {
			if !slices.Contains(constants.ReleasableProjects, project) {
				return fmt.Errorf("unknown project: %s (valid: %v)", project, constants.ReleasableProjects)
			}
		}
	} else {
		// Interactive selection
		selected, selectErr := selectProjectsInteractive(projects)
		if selectErr != nil {
			return fmt.Errorf("project selection cancelled: %w", selectErr)
		}

		selectedProjects = selected
	}

	// Step 4: Pre-flight checks (local repo status)
	if !skipConfirm {
		if err := runPreflightChecks(ctx, log, selectedProjects); err != nil {
			return err
		}
	}

	// Step 5: Single project: use existing flow with dependency check
	if len(selectedProjects) == 1 {
		return runSingleRelease(ctx, log, svc, projects, selectedProjects[0],
			bumpFlag, skipConfirm, watch, skipDeps, timeout)
	}

	// Step 5: Multiple projects: dependency-aware flow
	return runMultiRelease(ctx, log, svc, projects, selectedProjects,
		bumpFlag, skipConfirm, watch, skipDeps, timeout)
}

// runSingleRelease handles releasing a single project with dependency check.
func runSingleRelease(
	ctx context.Context,
	log logrus.FieldLogger,
	svc release.Service,
	projectInfos []release.ProjectInfo,
	project string,
	bumpFlag string,
	skipConfirm, watch, skipDeps bool,
	timeout time.Duration,
) error {
	// Check for missing dependencies (unless --no-deps)
	if !skipDeps {
		missing := release.CheckMissingDependencies([]string{project})
		if deps, ok := missing[project]; ok && len(deps) > 0 {
			// Prompt user about dependencies
			added, promptErr := promptMissingDependencies(project, deps, projectInfos)
			if promptErr != nil {
				return promptErr
			}

			if len(added) > 0 {
				// User chose to add dependencies - switch to multi-release
				allProjects := append(added, project)

				return runMultiRelease(ctx, log, svc, projectInfos, allProjects,
					bumpFlag, skipConfirm, watch, skipDeps, timeout)
			}
			// User chose to skip dependencies - continue with single release
		}
	}

	// Find selected project info
	var projectInfo *release.ProjectInfo

	for i := range projectInfos {
		if projectInfos[i].Name == project {
			projectInfo = &projectInfos[i]

			break
		}
	}

	if projectInfo == nil {
		return fmt.Errorf("project not found: %s", project)
	}

	ui.Info(fmt.Sprintf("Selected: %s (%s)", projectInfo.Name, projectInfo.Description))
	ui.Blank()

	// Handle release based on project type
	var result *release.ReleaseResult

	var err error

	if projectInfo.IsSemver {
		result, err = handleSemverRelease(ctx, svc, projectInfo, bumpFlag, skipConfirm)
	} else {
		result, err = handleWorkflowRelease(ctx, svc, projectInfo, skipConfirm)
	}

	if err != nil {
		return err
	}

	// Display result
	ui.Blank()
	ui.Success(fmt.Sprintf("Release triggered for %s", result.Project))

	if result.Version != "" {
		ui.Info(fmt.Sprintf("Version: %s", result.Version))
	}

	if result.WorkflowURL != "" {
		ui.Info(fmt.Sprintf("Workflow: %s", result.WorkflowURL))
	}

	// Watch build if requested
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

		if watchResult.Conclusion == release.StatusSuccess {
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

// promptMissingDependencies asks user if they want to release dependencies first.
func promptMissingDependencies(
	project string,
	missingDeps []string,
	projectInfos []release.ProjectInfo,
) ([]string, error) {
	// Find version info for the dependency
	var depVersion string

	for _, info := range projectInfos {
		if info.Name == missingDeps[0] { // Currently only one dep possible
			depVersion = info.CurrentVersion

			break
		}
	}

	ui.Blank()
	ui.Warning(fmt.Sprintf("%s depends on %s", project, missingDeps[0]))
	ui.Blank()
	fmt.Printf("  %s is currently at %s\n", missingDeps[0], pterm.Cyan(depVersion))
	ui.Blank()

	options := []ui.SelectOption{
		{
			Label:       fmt.Sprintf("No, use existing %s %s", missingDeps[0], depVersion),
			Description: fmt.Sprintf("%s will import the current %s version", project, missingDeps[0]),
			Value:       "skip",
		},
		{
			Label:       fmt.Sprintf("Yes, release %s first", missingDeps[0]),
			Description: fmt.Sprintf("Release and wait for %s build before %s", missingDeps[0], project),
			Value:       "add",
		},
	}

	selected, err := ui.Select(
		fmt.Sprintf("Does %s need changes from a new %s release?", project, missingDeps[0]),
		options,
	)
	if err != nil {
		return nil, err
	}

	if selected == "add" {
		return missingDeps, nil
	}

	return nil, nil
}

// runMultiRelease handles releasing multiple projects with dependency ordering.
func runMultiRelease(
	ctx context.Context,
	log logrus.FieldLogger,
	svc release.Service,
	projectInfos []release.ProjectInfo,
	projects []string,
	bumpFlag string,
	skipConfirm, watch, skipDeps bool,
	timeout time.Duration,
) error {
	// Build project info map
	infoMap := make(map[string]*release.ProjectInfo, len(projectInfos))
	for i := range projectInfos {
		infoMap[projectInfos[i].Name] = &projectInfos[i]
	}

	// Analyze dependencies and get release order
	ordered, deps, hasDeps := release.AnalyzeDependencies(projects)

	// Prompt for each dependency relationship if not skipping
	if hasDeps && !skipDeps && !skipConfirm {
		var promptErr error

		projects, promptErr = promptDependencyConfirmation(projects, ordered, deps, infoMap)
		if promptErr != nil {
			return promptErr
		}

		// Re-analyze with potentially updated project list
		ordered, deps, hasDeps = release.AnalyzeDependencies(projects)
	}

	// Show final dependency ordering if relevant
	if hasDeps {
		displayDependencyOrder(ordered, deps)
	}

	// Collect release configurations (in dependency order)
	configs := make([]release.ProjectReleaseConfig, 0, len(ordered))

	ui.Section("Release Configuration")
	ui.Blank()

	for _, project := range ordered {
		info, ok := infoMap[project]
		if !ok {
			ui.Warning(fmt.Sprintf("Skipping %s: project info not available", project))

			continue
		}

		config := release.ProjectReleaseConfig{
			Project: project,
			Info:    info,
		}

		if info.IsSemver {
			var bump release.BumpType

			if bumpFlag != "" {
				bump = release.BumpType(bumpFlag)
			} else {
				selected, selectErr := selectBumpType(project, info.CurrentVersion)
				if selectErr != nil {
					return selectErr
				}

				bump = selected
			}

			config.BumpType = bump

			currentVer, parseErr := release.ParseVersion(info.CurrentVersion)
			if parseErr != nil {
				ui.Warning(fmt.Sprintf("Could not parse version for %s: %v", project, parseErr))

				continue
			}

			config.NewVersion = currentVer.Bump(bump).String()
		}

		configs = append(configs, config)
	}

	if len(configs) == 0 {
		return fmt.Errorf("no projects to release")
	}

	// Display confirmation summary
	displayReleasePlan(configs, deps)

	if !skipConfirm {
		confirmed, confirmErr := ui.Confirm(fmt.Sprintf("Release %d project(s)?", len(configs)))
		if confirmErr != nil {
			return confirmErr
		}

		if !confirmed {
			ui.Info("Release cancelled")

			return nil
		}
	}

	// Split into phases
	phase1, phase2, phase3 := release.SplitByDependencyPhase(ordered, deps)

	report := release.NewReleaseReport()
	watchItems := make([]release.WatchItem, 0, len(configs))
	failedDeps := make(map[string]bool, len(configs))

	// Phase 1: Release dependencies and WAIT for their builds
	if len(phase1) > 0 {
		executePhase1(ctx, svc, phase1, configs, deps, report, failedDeps, watch, timeout)
	}

	// Phase 2: Release projects that depend on Phase 1 (skip if deps failed)
	if len(phase2) > 0 {
		watchItems = executePhase2(ctx, svc, phase2, configs, deps, report, failedDeps, watchItems)
	}

	// Phase 3: Release independent projects
	if len(phase3) > 0 {
		watchItems = executePhase3(ctx, svc, phase3, configs, report, watchItems)
	}

	// Watch remaining builds concurrently
	if watch && len(watchItems) > 0 {
		ui.Blank()
		ui.Header("Watching remaining builds")
		ui.Blank()

		opts := release.WatchOptions{
			Timeout:      timeout,
			PollInterval: 30 * time.Second,
		}

		spinner := ui.NewSpinner(fmt.Sprintf("Watching %d build(s)...", len(watchItems)))

		watchResult, _ := svc.WatchMultiple(ctx, watchItems, opts, func(project, status string) {
			log.WithFields(logrus.Fields{
				"project": project,
				"status":  status,
			}).Debug("Watch update")
		})

		report.UpdateFromMultiWatch(watchResult)

		if watchResult.AllSucceeded() {
			spinner.Success(fmt.Sprintf("All builds completed in %s", release.FormatDuration(watchResult.Duration())))
		} else {
			spinner.Warning("Some builds failed")
		}
	}

	report.Finalize()

	// Display final summary
	ui.Blank()
	displayReleaseSummary(report)

	if report.Failed() > 0 {
		return fmt.Errorf("%d release(s) failed", report.Failed())
	}

	return nil
}

// Helper functions

func findConfig(configs []release.ProjectReleaseConfig, project string) *release.ProjectReleaseConfig {
	for i := range configs {
		if configs[i].Project == project {
			return &configs[i]
		}
	}

	return nil
}

func executeRelease(
	ctx context.Context,
	svc release.Service,
	config *release.ProjectReleaseConfig,
) (*release.ReleaseResult, error) {
	spinner := ui.NewSpinner(fmt.Sprintf("Releasing %s...", config.Project))

	var result *release.ReleaseResult

	var err error

	if config.Info.IsSemver {
		result, err = svc.ReleaseSemver(ctx, config.Project, config.BumpType)
	} else {
		result, err = svc.ReleaseWorkflow(ctx, config.Project)
	}

	if err != nil {
		spinner.Fail(fmt.Sprintf("Failed to release %s: %v", config.Project, err))

		return nil, err
	}

	if config.Info.IsSemver {
		spinner.Success(fmt.Sprintf("Released %s %s", config.Project, config.NewVersion))
	} else {
		spinner.Success(fmt.Sprintf("Triggered %s build", config.Project))
	}

	return result, nil
}

func displayReleasePlan(configs []release.ProjectReleaseConfig, deps map[string]*release.DependencyInfo) {
	ui.Blank()
	ui.Section("Release Plan")
	ui.Blank()

	for i, cfg := range configs {
		var line string

		if cfg.Info.IsSemver {
			line = fmt.Sprintf("%d. %s: %s → %s (%s)",
				i+1, cfg.Project, cfg.Info.CurrentVersion, cfg.NewVersion, cfg.BumpType)
		} else {
			line = fmt.Sprintf("%d. %s: trigger docker build", i+1, cfg.Project)
		}

		fmt.Printf("  %s\n", line)

		// Show dependency info
		if info, ok := deps[cfg.Project]; ok {
			if info.NeedsWait {
				fmt.Printf("     %s\n", pterm.Gray("↳ Will wait for build to complete"))
			}

			if len(info.DependsOn) > 0 {
				fmt.Printf("     %s\n", pterm.Gray(fmt.Sprintf("↳ Depends on: %s",
					strings.Join(info.DependsOn, ", "))))
			}
		}
	}

	ui.Blank()
}

func displayReleaseSummary(report *release.ReleaseReport) {
	ui.Section("Release Summary")
	ui.Blank()

	// Build table data
	tableData := pterm.TableData{
		{"", "Project", "Version", "Duration", "Status"},
	}

	for _, entry := range report.Entries() {
		var symbol, status, version string

		// Determine the version to display
		displayVersion := entry.Version
		if displayVersion == "-" && entry.HeadSha != "" {
			// For workflow dispatch projects, show the commit SHA
			if len(entry.HeadSha) > 7 {
				displayVersion = entry.HeadSha[:7]
			} else {
				displayVersion = entry.HeadSha
			}
		}

		switch entry.Status {
		case release.StatusSuccess:
			symbol = pterm.Green("✓")
			status = pterm.Green("success")
			version = pterm.Green(displayVersion)
		case release.StatusFailed, release.StatusTimedOut:
			symbol = pterm.Red("✗")
			version = pterm.Gray(displayVersion)

			if entry.Error != "" {
				status = pterm.Red(entry.Error)
			} else {
				status = pterm.Red("failed")
			}
		case release.StatusSkipped:
			symbol = pterm.Yellow("⊘")
			version = pterm.Gray(displayVersion)
			status = pterm.Yellow(entry.Error)
		case release.StatusReleased:
			symbol = pterm.Cyan("→")
			version = pterm.Cyan(displayVersion)
			status = pterm.Cyan("triggered")
		default:
			symbol = pterm.Gray("○")
			version = pterm.Gray(displayVersion)
			status = pterm.Gray(entry.Status)
		}

		duration := "-"
		if entry.Duration > 0 {
			duration = release.FormatDuration(entry.Duration)
		}

		tableData = append(tableData, []string{symbol, entry.Project, version, duration, status})
	}

	// Render table
	_ = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()

	ui.Blank()

	// Summary line
	fmt.Printf("  %s\n", report.FormatSummaryLine())

	// Show copy-pasteable versions for successful releases
	successEntries := make([]release.ReportEntry, 0)

	for _, entry := range report.Entries() {
		if entry.Status == release.StatusSuccess || entry.Status == release.StatusReleased {
			// Include if has a version OR a HeadSha (for workflow dispatch projects)
			if (entry.Version != "" && entry.Version != "-") || entry.HeadSha != "" {
				successEntries = append(successEntries, entry)
			}
		}
	}

	if len(successEntries) > 0 {
		ui.Blank()
		ui.Info("Released versions:")

		for _, entry := range successEntries {
			// For workflow dispatch projects, show the full commit SHA
			displayVersion := entry.Version
			if (displayVersion == "" || displayVersion == "-") && entry.HeadSha != "" {
				displayVersion = entry.HeadSha
			}

			fmt.Printf("  %s: %s\n", pterm.Bold.Sprint(entry.Project), pterm.Cyan(displayVersion))

			if entry.URL != "" {
				fmt.Printf("    %s\n", pterm.Gray(entry.URL))
			}
		}
	}

	ui.Blank()
}

func selectProjectsInteractive(projectInfos []release.ProjectInfo) ([]string, error) {
	options := make([]ui.MultiSelectOption, 0, len(projectInfos))

	for _, info := range projectInfos {
		desc := fmt.Sprintf("(current: %s)", info.CurrentVersion)

		if !info.IsSemver {
			desc = "(triggers docker build)"
		}

		// Show dependency hint
		if deps := constants.GetDependencies(info.Name); len(deps) > 0 {
			desc += fmt.Sprintf(" [depends on: %s]", strings.Join(deps, ", "))
		}

		options = append(options, ui.MultiSelectOption{
			Label:       info.Name,
			Description: desc,
			Value:       info.Name,
		})
	}

	return ui.MultiSelect("Select projects to release", options)
}

func selectBumpType(project, currentVersion string) (release.BumpType, error) {
	options := []ui.SelectOption{
		{Label: "patch", Description: "Bug fixes (0.0.X)", Value: "patch"},
		{Label: "minor", Description: "New features (0.X.0)", Value: "minor"},
		{Label: "major", Description: "Breaking changes (X.0.0)", Value: "major"},
	}

	title := fmt.Sprintf("Version bump for %s (current: %s)", project, currentVersion)

	selected, err := ui.SelectWithDefault(title, options, "patch")
	if err != nil {
		return "", fmt.Errorf("bump selection cancelled: %w", err)
	}

	switch selected {
	case string(release.BumpPatch):
		return release.BumpPatch, nil
	case string(release.BumpMinor):
		return release.BumpMinor, nil
	case string(release.BumpMajor):
		return release.BumpMajor, nil
	default:
		return release.BumpPatch, nil
	}
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

// executePhase1 releases dependencies and waits for their builds.
func executePhase1(
	ctx context.Context,
	svc release.Service,
	phase1 []string,
	configs []release.ProjectReleaseConfig,
	deps map[string]*release.DependencyInfo,
	report *release.ReleaseReport,
	failedDeps map[string]bool,
	watch bool,
	timeout time.Duration,
) {
	ui.Blank()
	ui.Header("Phase 1: Releasing dependencies")
	ui.Blank()

	for _, project := range phase1 {
		config := findConfig(configs, project)
		if config == nil {
			continue
		}

		result, execErr := executeRelease(ctx, svc, config)
		if execErr != nil {
			report.AddRelease(&release.ReleaseResult{
				Project: project,
				Success: false,
				Error:   execErr,
			}, deps[project].DependsOn)
			failedDeps[project] = true

			continue
		}

		report.AddRelease(result, deps[project].DependsOn)

		// Wait for build to complete
		if result.RunID != "" && watch {
			spinner := ui.NewSpinner(fmt.Sprintf("Waiting for %s build...", project))

			opts := release.WatchOptions{
				Timeout:      timeout,
				PollInterval: 30 * time.Second,
			}

			watchResult, _ := svc.WatchRun(ctx, result.Repo, result.RunID, opts)

			report.UpdateWithWatchResult(project, watchResult)

			if watchResult.Conclusion == release.StatusSuccess {
				spinner.Success(fmt.Sprintf("%s build succeeded (%s)",
					project, release.FormatDuration(watchResult.Duration)))
			} else {
				spinner.Fail(fmt.Sprintf("%s build failed", project))
				failedDeps[project] = true
			}
		}
	}
}

// executePhase2 releases projects that depend on Phase 1 (skipping if deps failed).
func executePhase2(
	ctx context.Context,
	svc release.Service,
	phase2 []string,
	configs []release.ProjectReleaseConfig,
	deps map[string]*release.DependencyInfo,
	report *release.ReleaseReport,
	failedDeps map[string]bool,
	watchItems []release.WatchItem,
) []release.WatchItem {
	ui.Blank()
	ui.Header("Phase 2: Releasing dependent projects")
	ui.Blank()

	for _, project := range phase2 {
		// Check if any dependency failed
		depInfo := deps[project]
		skipReason := ""

		for _, dep := range depInfo.DependsOn {
			if failedDeps[dep] {
				skipReason = fmt.Sprintf("dependency %s failed", dep)

				break
			}
		}

		if skipReason != "" {
			ui.Warning(fmt.Sprintf("Skipping %s: %s", project, skipReason))

			config := findConfig(configs, project)
			version := "-"

			if config != nil && config.NewVersion != "" {
				version = config.NewVersion
			}

			report.AddSkipped(project, version, skipReason, depInfo.DependsOn)

			continue
		}

		config := findConfig(configs, project)
		if config == nil {
			continue
		}

		result, execErr := executeRelease(ctx, svc, config)
		if execErr != nil {
			report.AddRelease(&release.ReleaseResult{
				Project: project,
				Success: false,
				Error:   execErr,
			}, depInfo.DependsOn)

			continue
		}

		report.AddRelease(result, depInfo.DependsOn)

		if result.RunID != "" {
			watchItems = append(watchItems, release.WatchItem{
				Project: project,
				Repo:    result.Repo,
				RunID:   result.RunID,
			})
		}
	}

	return watchItems
}

// executePhase3 releases independent projects.
func executePhase3(
	ctx context.Context,
	svc release.Service,
	phase3 []string,
	configs []release.ProjectReleaseConfig,
	report *release.ReleaseReport,
	watchItems []release.WatchItem,
) []release.WatchItem {
	ui.Blank()
	ui.Header("Phase 3: Releasing independent projects")
	ui.Blank()

	for _, project := range phase3 {
		config := findConfig(configs, project)
		if config == nil {
			continue
		}

		result, execErr := executeRelease(ctx, svc, config)
		if execErr != nil {
			report.AddRelease(&release.ReleaseResult{
				Project: project,
				Success: false,
				Error:   execErr,
			}, nil)

			continue
		}

		report.AddRelease(result, nil)

		if result.RunID != "" {
			watchItems = append(watchItems, release.WatchItem{
				Project: project,
				Repo:    result.Repo,
				RunID:   result.RunID,
			})
		}
	}

	return watchItems
}

// promptDependencyConfirmation asks user about each dependency relationship.
// Returns the filtered list of projects after user choices.
func promptDependencyConfirmation(
	projects []string,
	ordered []string,
	deps map[string]*release.DependencyInfo,
	infoMap map[string]*release.ProjectInfo,
) ([]string, error) {
	projectsToRemove := make(map[string]bool)
	isFirstPrompt := true

	for _, project := range ordered {
		depInfo := deps[project]

		for _, dep := range depInfo.DependsOn {
			// Skip if we've already decided to remove the dependency
			if projectsToRemove[dep] {
				continue
			}

			// Show header on first dependency prompt
			if isFirstPrompt {
				ui.Section("Dependency Check")

				isFirstPrompt = false
			}

			// Find version info for the dependency
			depVersion := "unknown"
			if info, ok := infoMap[dep]; ok {
				depVersion = info.CurrentVersion
			}

			ui.Blank()
			ui.Warning(fmt.Sprintf("%s depends on %s", project, dep))
			ui.Blank()
			fmt.Printf("  %s is currently at %s\n", dep, pterm.Cyan(depVersion))
			ui.Blank()

			options := []ui.SelectOption{
				{
					Label:       fmt.Sprintf("Yes, release %s first", dep),
					Description: fmt.Sprintf("Release and wait for %s build before %s", dep, project),
					Value:       "keep",
				},
				{
					Label:       fmt.Sprintf("No, use existing %s %s", dep, depVersion),
					Description: fmt.Sprintf("%s will import the current %s version", project, dep),
					Value:       "skip",
				},
			}

			selected, selectErr := ui.Select(
				fmt.Sprintf("Does %s need changes from a new %s release?", project, dep),
				options,
			)
			if selectErr != nil {
				return nil, selectErr
			}

			if selected == "skip" {
				projectsToRemove[dep] = true
			}
		}
	}

	// Remove skipped dependencies from the project list
	if len(projectsToRemove) > 0 {
		filteredProjects := make([]string, 0, len(projects))

		for _, p := range projects {
			if !projectsToRemove[p] {
				filteredProjects = append(filteredProjects, p)
			}
		}

		return filteredProjects, nil
	}

	return projects, nil
}

// displayDependencyOrder shows the final release order to the user.
func displayDependencyOrder(ordered []string, deps map[string]*release.DependencyInfo) {
	ui.Blank()
	ui.Info("Release order (dependencies first):")

	for _, project := range ordered {
		info := deps[project]
		if len(info.DependsOn) > 0 {
			fmt.Printf("  • %s (depends on %s)\n", project, strings.Join(info.DependsOn, ", "))
		} else if info.NeedsWait {
			fmt.Printf("  • %s (build must complete first)\n", project)
		} else {
			fmt.Printf("  • %s\n", project)
		}
	}

	ui.Blank()
}

// runPreflightChecks performs pre-release validation on local repositories.
func runPreflightChecks(
	ctx context.Context,
	log logrus.FieldLogger,
	selectedProjects []string,
) error {
	// Try to find and load config to get repo paths
	configPath := config.FindConfig("")
	if configPath == "" {
		log.Debug("no config found for pre-flight checks (skipping local repo checks)")

		return nil
	}

	labCfg, _, err := config.LoadLabConfig(configPath)
	if err != nil {
		log.WithError(err).Debug("could not load lab config for pre-flight checks (skipping local repo checks)")

		return nil
	}

	// Build map of project -> repo path
	repoPaths := getRepoPathsFromLabConfig(labCfg, selectedProjects)
	if len(repoPaths) == 0 {
		return nil
	}

	// Check git status for each repo (includes git fetch, which can be slow)
	spinner := ui.NewSpinner("Checking local repository status...")

	checker := git.NewChecker(log)
	statuses := checker.CheckRepositories(ctx, repoPaths)

	spinner.Success("Repository status checked")

	// Analyze results
	hasWarnings := false
	uncommittedRepos := make([]git.RepoStatus, 0)
	noChangesRepos := make([]git.RepoStatus, 0)

	for _, status := range statuses {
		if status.Error != nil {
			continue
		}

		if status.HasUncommitted {
			uncommittedRepos = append(uncommittedRepos, status)
			hasWarnings = true
		}

		if status.CommitsSinceTag == 0 && status.LatestTag != "" {
			noChangesRepos = append(noChangesRepos, status)
		}
	}

	if !hasWarnings && len(noChangesRepos) == 0 {
		return nil
	}

	// Display pre-flight checklist
	ui.Section("Pre-release Checklist")
	ui.Blank()

	// Show uncommitted changes warning
	if len(uncommittedRepos) > 0 {
		fmt.Printf("  %s %s\n", pterm.Red("⚠"), pterm.Red("Uncommitted changes detected:"))

		for _, repo := range uncommittedRepos {
			fmt.Printf("    • %s: %d uncommitted file(s)\n",
				pterm.Yellow(repo.Name), repo.UncommittedCount)
		}

		ui.Blank()
	}

	// Show repos with no changes since last release
	if len(noChangesRepos) > 0 {
		fmt.Printf("  %s %s\n", pterm.Yellow("○"), pterm.Yellow("No commits since last release:"))

		for _, repo := range noChangesRepos {
			fmt.Printf("    • %s: last release %s (0 new commits)\n",
				repo.Name, pterm.Cyan(repo.LatestTag))
		}

		ui.Blank()
	}

	// Show reminder checklist
	fmt.Println(pterm.Gray("  ─────────────────────────────────────────────────"))
	ui.Blank()
	fmt.Println("  Before releasing, please verify:")
	ui.Blank()

	// Build list of repos for the checklist
	repoNames := make([]string, 0, len(repoPaths))
	for name := range repoPaths {
		repoNames = append(repoNames, name)
	}

	fmt.Printf("    □ Code changes merged to master on: %s\n", pterm.Cyan(strings.Join(repoNames, ", ")))
	fmt.Println("    □ All CI checks passing on master branches")
	fmt.Println("    □ Database migrations applied (if applicable)")
	ui.Blank()
	fmt.Println(pterm.Gray("  ─────────────────────────────────────────────────"))
	ui.Blank()

	// Require explicit confirmation
	confirmed, confirmErr := ui.Confirm("I have verified the above and want to proceed")
	if confirmErr != nil {
		return confirmErr
	}

	if !confirmed {
		return fmt.Errorf("release cancelled: pre-flight checks not confirmed")
	}

	ui.Blank()

	return nil
}

// getRepoPathsFromLabConfig extracts repo paths for selected projects from lab config.
func getRepoPathsFromLabConfig(labCfg *config.LabConfig, projects []string) map[string]string {
	paths := make(map[string]string)

	if labCfg == nil {
		return paths
	}

	repos := labCfg.Repos

	for _, project := range projects {
		switch project {
		case constants.ProjectCBT:
			if repos.CBT != "" {
				paths[project] = repos.CBT
			}
		case constants.ProjectXatuCBT:
			if repos.XatuCBT != "" {
				paths[project] = repos.XatuCBT
			}
		case constants.ProjectCBTAPI:
			if repos.CBTAPI != "" {
				paths[project] = repos.CBTAPI
			}
		case constants.ProjectLabBackend:
			if repos.LabBackend != "" {
				paths[project] = repos.LabBackend
			}
		case constants.ProjectLab:
			if repos.Lab != "" {
				paths[project] = repos.Lab
			}
		}
	}

	return paths
}
