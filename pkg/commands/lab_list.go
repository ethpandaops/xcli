package commands

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ethpandaops/xcli/pkg/instance"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/spf13/cobra"
)

// tableColumnValue is the "Value" table column header.
const tableColumnValue = "Value"

// NewLabListCommand creates the lab list command.
func NewLabListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List known lab instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := instance.DefaultRegistry()
			if err != nil {
				return err
			}

			result, err := instance.NewReconciler(registry).ReconcileAll(cmd.Context())
			if err != nil {
				return err
			}

			if len(result.Instances) == 0 {
				ui.Info("No lab instances registered")

				if result.DockerError != nil {
					ui.Warning(fmt.Sprintf("Docker reconciliation skipped: %v", result.DockerError))
				}

				return nil
			}

			printInstanceList(result.Instances)

			if result.DockerError != nil {
				ui.Warning(fmt.Sprintf("Docker reconciliation skipped: %v", result.DockerError))
			}

			return nil
		},
	}
}

// NewLabShowCommand creates the lab show command.
func NewLabShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <instance-id>",
		Short: "Show one lab instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceID, err := instance.SanitizeID(args[0])
			if err != nil {
				return err
			}

			registry, err := instance.DefaultRegistry()
			if err != nil {
				return err
			}

			result, err := instance.NewReconciler(registry).ReconcileAll(cmd.Context())
			if err != nil {
				return err
			}

			for _, item := range result.Instances {
				if item.InstanceID == instanceID {
					printInstanceDetails(item)

					if result.DockerError != nil {
						ui.Warning(fmt.Sprintf("Docker reconciliation skipped: %v", result.DockerError))
					}

					return nil
				}
			}

			return fmt.Errorf("instance %q not found", instanceID)
		},
	}
}

func printInstanceList(instances []*instance.ReconciledInstance) {
	for i, item := range instances {
		if i > 0 {
			ui.Blank()
		}

		manifest := item.Manifest
		ui.Header(fmt.Sprintf("%s (%s)", item.InstanceID, item.Status))
		ui.Info(fmt.Sprintf("Root: %s", manifest.RootDir))
		ui.Info(fmt.Sprintf("Config: %s", manifest.ConfigPath))

		repoNames := make([]string, 0, len(manifest.Repos))
		for name := range manifest.Repos {
			repoNames = append(repoNames, name)
		}

		sort.Strings(repoNames)

		rows := make([][]string, 0, len(repoNames))
		for _, name := range repoNames {
			repo := manifest.Repos[name]
			rows = append(rows, []string{
				name,
				repo.Branch,
				shortCommit(repo.Commit),
				dirtyLabel(repo.Dirty),
				repo.Path,
			})
		}

		if len(rows) > 0 {
			ui.Table([]string{"Repo", "Branch", "Commit", "Dirty", "Path"}, rows)
		}
	}
}

func printInstanceDetails(item *instance.ReconciledInstance) {
	manifest := item.Manifest

	ui.Header(fmt.Sprintf("%s (%s)", item.InstanceID, item.Status))
	ui.Info(fmt.Sprintf("Registry: %t", item.RegistryPresent))
	ui.Info(fmt.Sprintf("Root: %s", manifest.RootDir))
	ui.Info(fmt.Sprintf("Config: %s", manifest.ConfigPath))
	ui.Info(fmt.Sprintf("Overrides: %s", manifest.OverridesPath))
	ui.Info(fmt.Sprintf("State dir: %s", manifest.StateDir))
	ui.Info(fmt.Sprintf("Mode: %s", manifest.Mode))

	if manifest.LastError != "" {
		ui.Warning(fmt.Sprintf("Last error: %s", manifest.LastError))
	}

	if len(manifest.URLs) > 0 {
		rows := sortedMapRows(manifest.URLs)

		ui.Blank()
		ui.Table([]string{"Service", "URL"}, rows)
	}

	if ports := manifest.Ports.NamedPorts(); len(ports) > 0 {
		names := sortedKeysInt(ports)

		rows := make([][]string, 0, len(names))
		for _, name := range names {
			port := ports[name]
			if port == 0 {
				continue
			}

			rows = append(rows, []string{
				name,
				strconv.Itoa(port),
				boolLabel(item.Live.Ports[name]),
			})
		}

		if len(rows) > 0 {
			ui.Blank()
			ui.Table([]string{"Port", tableColumnValue, "Bound"}, rows)
		}
	}

	if len(manifest.Docker.Containers) > 0 || len(manifest.Docker.Volumes) > 0 || len(item.Live.DockerResources) > 0 {
		rows := make([][]string, 0, len(manifest.Docker.Containers)+len(manifest.Docker.Volumes)+len(item.Live.DockerResources))
		for _, name := range sortedKeysString(manifest.Docker.Containers) {
			rows = append(rows, []string{"container", name, manifest.Docker.Containers[name], "-"})
		}

		for _, name := range sortedKeysString(manifest.Docker.Volumes) {
			rows = append(rows, []string{"volume", name, manifest.Docker.Volumes[name], "-"})
		}

		for _, resource := range item.Live.DockerResources {
			rows = append(rows, []string{resource.Kind, "live", resource.Name, resource.State})
		}

		ui.Blank()
		ui.Table([]string{"Type", "Service", "Name", "State"}, rows)
	}

	printRepoTable(manifest)
}

func printRepoTable(manifest *instance.Manifest) {
	repoNames := make([]string, 0, len(manifest.Repos))
	for name := range manifest.Repos {
		repoNames = append(repoNames, name)
	}

	sort.Strings(repoNames)

	rows := make([][]string, 0, len(repoNames))
	for _, name := range repoNames {
		repo := manifest.Repos[name]
		rows = append(rows, []string{
			name,
			repo.Branch,
			shortCommit(repo.Commit),
			dirtyLabel(repo.Dirty),
			repo.Path,
		})
	}

	if len(rows) > 0 {
		ui.Blank()
		ui.Table([]string{"Repo", "Branch", "Commit", "Dirty", "Path"}, rows)
	}
}

func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) <= 12 {
		return commit
	}

	return commit[:12]
}

func dirtyLabel(dirty bool) string {
	if dirty {
		return "dirty"
	}

	return "clean"
}

func sortedMapRows(values map[string]string) [][]string {
	keys := sortedKeysString(values)

	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, []string{key, values[key]})
	}

	return rows
}

func sortedKeysString(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func sortedKeysInt(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func boolLabel(value bool) string {
	if value {
		return "yes"
	}

	return "no"
}
