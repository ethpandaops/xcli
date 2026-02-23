package commands

import (
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/spf13/cobra"
)

// completeModes returns a ValidArgsFunction that completes mode values.
func completeModes() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return []string{constants.ModeLocal, constants.ModeHybrid}, cobra.ShellCompDirectiveNoFileComp
	}
}

// completeReleasableProjects returns a ValidArgsFunction that completes releasable project names.
// Supports multiple arguments since release accepts multiple projects.
func completeReleasableProjects() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		alreadyUsed := make(map[string]bool, len(args))
		for _, arg := range args {
			alreadyUsed[arg] = true
		}

		completions := make([]string, 0, len(constants.ReleasableProjects))
		for _, project := range constants.ReleasableProjects {
			if !alreadyUsed[project] {
				completions = append(completions, project)
			}
		}

		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}
