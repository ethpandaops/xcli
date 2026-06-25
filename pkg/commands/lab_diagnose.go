package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ethpandaops/xcli/pkg/ai"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/diagnostic"
	"github.com/ethpandaops/xcli/pkg/instance"
	"github.com/ethpandaops/xcli/pkg/portutil"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/ethpandaops/xcli/pkg/workspace"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabDiagnoseCommand creates the lab diagnose command.
func NewLabDiagnoseCommand(
	log logrus.FieldLogger,
	configPath string,
	instanceOverride *string,
) *cobra.Command {
	var (
		useAI    bool
		provider string
		reportID string
	)

	cmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Diagnose the selected lab instance and latest build failure",
		Long: `Analyze the selected lab instance and the most recent build failure.

The instance section shows the authoritative config, overrides, state path,
instance id, ports, reconciled status, and common multi-instance traps.

By default, uses pattern matching for instant results.
Use --ai flag to get AI-powered analysis from Claude Code.

Examples:
  xcli lab diagnose                    # Show instance state and latest failure
  xcli lab --instance alpha diagnose   # Diagnose another instance
  xcli lab diagnose --ai               # Use Claude Code for AI analysis
  xcli lab diagnose --id xxx           # Diagnose specific report by ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			diag, err := buildLabDiagnosticContext(
				cmd.Context(),
				configPath,
				instanceOverrideValue(instanceOverride),
			)
			if err != nil {
				return err
			}

			printLabDiagnostics(diag)

			store := diagnostic.NewStore(log, filepath.Join(diag.Runtime.Manifest.StateDir, "errors"))

			// Load report (latest or by ID)
			var report *diagnostic.RebuildReport

			if reportID != "" {
				report, err = store.Load(reportID)
			} else {
				report, err = store.Latest()
			}

			if err != nil {
				ui.Info(fmt.Sprintf("No diagnostic reports found: %v", err))

				return nil
			}

			// Check for failures
			if !report.HasFailures() {
				ui.Success("No failures in the selected report")
				ui.Info(fmt.Sprintf("Report ID: %s | %d steps completed successfully",
					report.ID, report.TotalCount))

				return nil
			}

			// Display failure summary first
			ui.DisplayFailureSummary(report)

			// Try AI diagnosis if requested
			if useAI {
				engine, engineErr := ai.NewEngine(ai.ProviderID(provider), log)
				if engineErr != nil {
					ui.Warning("AI provider is not available: " + engineErr.Error())
					ui.Info("Falling back to pattern matching...")

					useAI = false
				} else if !engine.IsAvailable() {
					ui.Warning(fmt.Sprintf("AI provider %q is not available", provider))
					ui.Info("Falling back to pattern matching...")

					useAI = false
				} else {
					spinner := ui.NewSpinner(fmt.Sprintf("Analyzing with %s...", provider))
					prompt := diagnostic.BuildReportPrompt(report)

					response, diagErr := engine.Ask(cmd.Context(), prompt)
					if diagErr != nil {
						spinner.Fail("AI diagnosis failed")
						ui.Warning(diagErr.Error())
						ui.Info("Falling back to pattern matching...")

						useAI = false
					} else {
						diagnosis := diagnostic.ParseDiagnosisResponse(response)

						spinner.Success("Analysis complete")
						ui.DisplayAIDiagnosis(diagnosis)

						return nil
					}
				}
			}

			// Pattern matching fallback
			matcher := diagnostic.NewPatternMatcher()

			for _, result := range report.Failed() {
				diag := matcher.Match(&result)
				if diag != nil && diag.Matched {
					ui.DisplayPatternDiagnosis(&result, diag)
				} else {
					ui.DisplayRawError(&result)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&useAI, "ai", false, "Use AI for diagnosis")
	cmd.Flags().StringVar(&provider, "provider", string(ai.DefaultProvider), "AI provider to use")
	cmd.Flags().StringVar(&reportID, "id", "", "Diagnose specific report by ID")

	return cmd
}

type labDiagnosticContext struct {
	Runtime        *instance.Runtime
	Reconciled     *instance.ReconciledInstance
	Traps          []string
	PortChecks     map[int]*portutil.PortConflict
	ManifestLoaded bool
}

type diagnosticPortConflict struct {
	Name    string
	Port    int
	PID     int
	Process string
}

func instanceOverrideValue(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

func buildLabDiagnosticContext(
	ctx context.Context,
	configPath string,
	cliInstanceID string,
) (*labDiagnosticContext, error) {
	labCfg, ws, err := workspace.LoadLabConfig(configPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	instanceID, err := instance.ResolveID(ws, labCfg, cliInstanceID)
	if err != nil {
		return nil, err
	}

	registry, err := instance.DefaultRegistry()
	if err != nil {
		return nil, err
	}

	var (
		runtime        *instance.Runtime
		manifestLoaded bool
		traps          []string
	)

	manifest, loadErr := registry.Load(instanceID)
	if loadErr == nil {
		runtime, err = instance.NewRuntimeFromManifestConfig(ctx, manifest, registry)
		if err != nil {
			return nil, err
		}

		manifestLoaded = true
	} else {
		if !os.IsNotExist(loadErr) {
			traps = append(traps, fmt.Sprintf("Could not load registry manifest for %s: %v", instanceID, loadErr))
		}

		runtime, err = instance.NewRuntimeFromWorkspace(ctx, ws, labCfg, cliInstanceID, instance.RuntimeOptions{
			Registry:   registry,
			ClaimPorts: false,
			ProbePorts: false,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to resolve candidate runtime: %w", err)
		}

		traps = append(traps, "No persisted manifest for this instance yet; run 'xcli lab up' to create one.")
	}

	if runtime.Workspace != nil {
		ws = runtime.Workspace
	}

	if runtime.LabConfig != nil {
		labCfg = runtime.LabConfig
	}

	if stray, ok, strayErr := detectStrayCWDOverrides(ws); strayErr != nil {
		traps = append(traps, fmt.Sprintf("Could not inspect current-directory overrides: %v", strayErr))
	} else if ok {
		traps = append(traps, fmt.Sprintf(
			"Current directory has %s, but authoritative overrides are %s",
			stray,
			ws.OverridesPath,
		))
	}

	traps = append(traps, missingRepoPaths(labCfg)...)

	var reconciled *instance.ReconciledInstance

	result, reconcileErr := instance.NewReconciler(registry).ReconcileAll(ctx)
	if reconcileErr != nil {
		traps = append(traps, fmt.Sprintf("Could not reconcile instances: %v", reconcileErr))
	} else {
		for _, item := range result.Instances {
			if item.InstanceID == runtime.InstanceID {
				reconciled = item

				break
			}
		}

		if result.DockerError != nil {
			traps = append(traps, fmt.Sprintf("Docker reconciliation skipped: %v", result.DockerError))
		}
	}

	portChecks := probeRuntimePorts(runtime)

	portConflicts := nonOwnedPortConflicts(runtime, portChecks)
	for _, conflict := range portConflicts {
		owner := fmt.Sprintf("PID %d", conflict.PID)
		if conflict.Process != "" {
			owner += " " + conflict.Process
		}

		traps = append(traps, fmt.Sprintf(
			"Port %d (%s) is bound by %s and is not recorded as this instance app PID.",
			conflict.Port,
			conflict.Name,
			owner,
		))
	}

	return &labDiagnosticContext{
		Runtime:        runtime,
		Reconciled:     reconciled,
		Traps:          traps,
		PortChecks:     portChecks,
		ManifestLoaded: manifestLoaded,
	}, nil
}

func printLabDiagnostics(diag *labDiagnosticContext) {
	runtime := diag.Runtime
	manifest := runtime.Manifest

	status := "unregistered"
	if diag.Reconciled != nil {
		status = diag.Reconciled.Status
	} else if manifest.Status != "" {
		status = manifest.Status
	}

	ui.Header("Instance Diagnostics")
	ui.Table([]string{"Field", tableColumnValue}, [][]string{
		{"Instance", runtime.InstanceID},
		{"Status", status},
		{"Manifest", boolLabel(diag.ManifestLoaded)},
		{"Config", manifest.ConfigPath},
		{"Overrides", manifest.OverridesPath},
		{"State dir", manifest.StateDir},
		{"Root", manifest.RootDir},
		{"Mode", manifest.Mode},
	})

	if ports := runtime.Ports.NamedPorts(); len(ports) > 0 {
		names := sortedKeysInt(ports)

		rows := make([][]string, 0, len(names))
		for _, name := range names {
			port := ports[name]
			if port == 0 {
				continue
			}

			bound := "no"
			owner := "-"

			if conflict := diag.PortChecks[port]; conflict != nil && conflict.PID > 0 {
				bound = "yes"

				owner = fmt.Sprintf("PID %d", conflict.PID)
				if conflict.Process != "" {
					owner += " " + conflict.Process
				}
			}

			rows = append(rows, []string{name, strconv.Itoa(port), bound, owner})
		}

		if len(rows) > 0 {
			ui.Blank()
			ui.Table([]string{"Port", tableColumnValue, "Bound", "Owner"}, rows)
		}
	}

	if len(diag.Traps) == 0 {
		ui.Blank()
		ui.Success("No multi-instance traps detected")

		return
	}

	ui.Blank()
	ui.Header("Traps")

	for _, trap := range diag.Traps {
		ui.Warning(trap)
	}
}

func detectStrayCWDOverrides(ws *workspace.Workspace) (string, bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, err
	}

	cwdOverrides, err := filepath.Abs(filepath.Join(cwd, constants.CBTOverridesFile))
	if err != nil {
		return "", false, err
	}

	if filepath.Clean(cwdOverrides) == filepath.Clean(ws.OverridesPath) {
		return "", false, nil
	}

	if _, err := os.Stat(cwdOverrides); err == nil {
		return cwdOverrides, true, nil
	} else if !os.IsNotExist(err) {
		return "", false, err
	}

	return "", false, nil
}

func missingRepoPaths(labCfg *config.LabConfig) []string {
	missing := make([]string, 0)

	for _, repo := range labCfg.Repos.Ordered() {
		name := repo.Name

		path := strings.TrimSpace(repo.Path)
		if path == "" {
			missing = append(missing, fmt.Sprintf("Repo %s has no configured path", name))

			continue
		}

		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, fmt.Sprintf("Repo %s path does not exist: %s", name, path))
			} else {
				missing = append(missing, fmt.Sprintf("Repo %s path could not be inspected: %s (%v)", name, path, err))
			}
		}
	}

	return missing
}

func probeRuntimePorts(runtime *instance.Runtime) map[int]*portutil.PortConflict {
	ports := runtime.Ports.NamedPorts()

	checks := make(map[int]*portutil.PortConflict, len(ports))
	for _, port := range ports {
		if port <= 0 {
			continue
		}

		checks[port] = portutil.CheckPort(port)
	}

	return checks
}

func nonOwnedPortConflicts(
	runtime *instance.Runtime,
	portChecks map[int]*portutil.PortConflict,
) []diagnosticPortConflict {
	ownedPIDs := make(map[int]bool)

	if runtime.Manifest != nil {
		for _, pid := range runtime.Manifest.PIDs {
			if pid > 0 {
				ownedPIDs[pid] = true
			}
		}
	}

	ports := runtime.Ports.NamedPorts()
	names := sortedKeysInt(ports)
	conflicts := make([]diagnosticPortConflict, 0)

	for _, name := range names {
		port := ports[name]
		if port <= 0 {
			continue
		}

		conflict := portChecks[port]
		if conflict == nil || conflict.PID <= 0 || ownedPIDs[conflict.PID] {
			continue
		}

		process := strings.TrimSpace(conflict.Process)
		if process != "" {
			process = "(" + process + ")"
		}

		conflicts = append(conflicts, diagnosticPortConflict{
			Name:    name,
			Port:    port,
			PID:     conflict.PID,
			Process: process,
		})
	}

	return conflicts
}
