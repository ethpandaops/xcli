package commands

import (
	"io"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// completeServices returns a ValidArgsFunction that completes service names.
// It loads the config and creates an orchestrator to get the dynamic service list.
func completeServices(configPath string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		// Don't complete if we already have an argument (for single-arg commands)
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Create a silent logger for completion (no output)
		log := logrus.New()
		log.SetOutput(io.Discard)

		// Try to load config - fail gracefully
		labCfg, cfgPath, err := config.LoadLabConfig(configPath)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Try to create orchestrator - fail gracefully
		orch, err := orchestrator.NewOrchestrator(log, labCfg, cfgPath)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return orch.GetValidServices(), cobra.ShellCompDirectiveNoFileComp
	}
}

// completeModes returns a ValidArgsFunction that completes mode values.
func completeModes() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return []string{constants.ModeLocal, constants.ModeHybrid}, cobra.ShellCompDirectiveNoFileComp
	}
}

// completeRebuildProjects returns a ValidArgsFunction that completes rebuild project names.
func completeRebuildProjects() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return []string{
			"xatu-cbt",
			"all",
			"cbt",
			"cbt-api",
			"lab-backend",
			"lab-frontend",
			"prometheus",
			"grafana",
		}, cobra.ShellCompDirectiveNoFileComp
	}
}

// completeReleasableProjects returns a ValidArgsFunction that completes releasable project names.
// Supports multiple arguments since release accepts multiple projects.
func completeReleasableProjects() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		// Filter out already-provided arguments
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
