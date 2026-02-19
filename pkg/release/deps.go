package release

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ethpandaops/xcli/pkg/constants"
)

// DependencyInfo describes a dependency relationship for display.
type DependencyInfo struct {
	Project    string   // The project being analyzed
	DependsOn  []string // Projects this project depends on
	RequiredBy []string // Projects that depend on this project
	NeedsWait  bool     // True if we need to wait for this project's build
}

// AnalyzeDependencies examines the selected projects and returns dependency info.
// Returns:
// - ordered: projects in release order (dependencies first)
// - deps: map of project to its dependency info
// - hasDeps: true if there are any dependency relationships in selection.
func AnalyzeDependencies(selected []string) (ordered []string, deps map[string]*DependencyInfo, hasDeps bool) {
	deps = make(map[string]*DependencyInfo, len(selected))

	// Initialize dependency info for each selected project
	for _, project := range selected {
		deps[project] = &DependencyInfo{
			Project:    project,
			DependsOn:  make([]string, 0, 2),
			RequiredBy: make([]string, 0, 2),
		}
	}

	// Populate dependency relationships (only for selected projects)
	for _, project := range selected {
		projectDeps := constants.GetDependencies(project)
		for _, dep := range projectDeps {
			// Only consider if dependency is also selected
			if slices.Contains(selected, dep) {
				deps[project].DependsOn = append(deps[project].DependsOn, dep)
				deps[dep].RequiredBy = append(deps[dep].RequiredBy, project)
				deps[dep].NeedsWait = true // Must wait for this build to complete
				hasDeps = true
			}
		}
	}

	// Topological sort for release order
	ordered = topologicalSort(selected, deps)

	return ordered, deps, hasDeps
}

// topologicalSort returns projects in dependency order (dependencies first).
func topologicalSort(projects []string, deps map[string]*DependencyInfo) []string {
	result := make([]string, 0, len(projects))
	visited := make(map[string]bool, len(projects))

	var visit func(string)

	visit = func(project string) {
		if visited[project] {
			return
		}

		visited[project] = true

		// Visit dependencies first
		if info, ok := deps[project]; ok {
			for _, dep := range info.DependsOn {
				visit(dep)
			}
		}

		result = append(result, project)
	}

	for _, project := range projects {
		visit(project)
	}

	return result
}

// CheckMissingDependencies checks if any selected project depends on
// a project that is NOT selected. Returns info for prompting user.
// Returns: map of dependent project -> missing dependencies.
func CheckMissingDependencies(selected []string) map[string][]string {
	missing := make(map[string][]string, len(selected))

	for _, project := range selected {
		projectDeps := constants.GetDependencies(project)
		for _, dep := range projectDeps {
			if !slices.Contains(selected, dep) {
				if missing[project] == nil {
					missing[project] = make([]string, 0, 1)
				}

				missing[project] = append(missing[project], dep)
			}
		}
	}

	return missing
}

// SplitByDependencyPhase splits projects into phases for release:
// - Phase 1: Projects that others depend on (release and wait for build)
// - Phase 2: Projects that depend on Phase 1 (release after Phase 1 completes)
// - Phase 3: Independent projects (can release anytime, watch concurrently).
func SplitByDependencyPhase(
	ordered []string,
	deps map[string]*DependencyInfo,
) (phase1, phase2, phase3 []string) {
	phase1 = make([]string, 0, len(ordered))
	phase2 = make([]string, 0, len(ordered))
	phase3 = make([]string, 0, len(ordered))

	for _, project := range ordered {
		info := deps[project]
		if info.NeedsWait {
			// This project is depended on - must release and wait first
			phase1 = append(phase1, project)
		} else if len(info.DependsOn) > 0 {
			// This project depends on others - release after dependencies complete
			phase2 = append(phase2, project)
		} else {
			// Independent project
			phase3 = append(phase3, project)
		}
	}

	return phase1, phase2, phase3
}

// FormatDependencyMessage creates a human-readable message about dependencies.
func FormatDependencyMessage(project string, dependsOn []string) string {
	if len(dependsOn) == 0 {
		return ""
	}

	if len(dependsOn) == 1 {
		return fmt.Sprintf("%s depends on %s", project, dependsOn[0])
	}

	return fmt.Sprintf("%s depends on %s", project, strings.Join(dependsOn, ", "))
}
