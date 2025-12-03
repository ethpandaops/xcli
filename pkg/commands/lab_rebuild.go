package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/diagnostic"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabRebuildCommand creates the lab rebuild command.
func NewLabRebuildCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "rebuild [project]",
		Short: "Rebuild specific components during active development with automatic service restarts",
		Long: `Rebuild specific components during active development with automatic service restarts.

Purpose:
  This command is designed for rapid iteration during local development.
  It rebuilds ONLY what changed and automatically restarts affected services.

Use cases:
  • You modified code and need to test changes immediately
  • You added/changed models in xatu-cbt and need full regeneration
  • You updated API endpoints in cbt-api
  • Fast development loop without full 'down && up' cycle

Key difference from 'build':
  • build  = Build everything, don't start services (CI/CD)
  • rebuild = Build specific component + restart its services (development)

Supported projects:
  xatu-cbt     - Full rebuild and restart of ALL services
  all          - Same as 'xatu-cbt' - full rebuild and restart
                 Rebuilds: xatu-cbt, cbt, lab-backend, cbt-api
                 Restarts: cbt, cbt-api, lab-backend, lab-frontend
                 Use when: You want complete rebuild with all changes applied

  cbt          - Rebuild CBT binary + restart all CBT services
                 Use when: You modify CBT engine code

  cbt-api      - Regenerate protos + rebuild + restart all cbt-api services
                 Use when: You modify cbt-api endpoints

  lab-backend  - Rebuild + restart lab-backend service
                 Use when: You modify lab-backend code

  lab-frontend - Regenerate API types + restart lab-frontend
                 Use when: cbt-api OpenAPI spec changed

Examples:
  xcli lab rebuild all               # Full rebuild and restart (alias for xatu-cbt)
  xcli lab rebuild xatu-cbt          # Full model update (same as 'all')
  xcli lab rebuild cbt               # Quick CBT engine iteration
  xcli lab rebuild lab-backend -v    # Rebuild with verbose output

Note: All rebuild commands automatically restart their respective services if running.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project := args[0]

			// Load config
			labCfg, cfgPath, err := config.LoadLabConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Compute stateDir from config path (same logic as orchestrator)
			absConfigPath, err := filepath.Abs(cfgPath)
			if err != nil {
				return fmt.Errorf("failed to get absolute config path: %w", err)
			}

			configDir := filepath.Dir(absConfigPath)
			stateDir := filepath.Join(configDir, ".xcli")

			// Create orchestrator
			orch, err := orchestrator.NewOrchestrator(log, labCfg, cfgPath)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			// Set verbose mode
			orch.SetVerbose(verbose)

			// Use the command's context for cancellation support
			ctx := cmd.Context()

			// Route to appropriate build
			switch project {
			case "xatu-cbt", "all":
				return runFullRebuild(ctx, log, orch, verbose, stateDir)

			case "cbt":
				spinner := ui.NewSpinner("Rebuilding CBT")

				if err := orch.Builder().BuildCBT(ctx, true); err != nil {
					spinner.Fail("Failed to rebuild CBT")

					return fmt.Errorf("failed to rebuild CBT: %w", err)
				}

				spinner.Success("CBT rebuilt successfully")

				// Restart all CBT services
				enabledNetworks := orch.Config().EnabledNetworks()
				for _, network := range enabledNetworks {
					serviceName := fmt.Sprintf("cbt-%s", network.Name)
					spinner = ui.NewSpinner(fmt.Sprintf("Restarting %s", serviceName))

					if err := orch.Restart(ctx, serviceName); err != nil {
						spinner.Warning(fmt.Sprintf("Could not restart %s", serviceName))
					} else {
						spinner.Success(fmt.Sprintf("%s restarted", serviceName))
					}
				}

			case "cbt-api":
				spinner := ui.NewSpinner("Regenerating protos and rebuilding cbt-api")

				if err := orch.Builder().BuildCBTAPI(ctx, true); err != nil {
					spinner.Fail("Failed to rebuild cbt-api")

					return fmt.Errorf("failed to rebuild cbt-api: %w", err)
				}

				spinner.Success("cbt-api rebuilt successfully")

				// Restart all cbt-api services
				enabledNetworks := orch.Config().EnabledNetworks()
				for _, network := range enabledNetworks {
					serviceName := fmt.Sprintf("cbt-api-%s", network.Name)
					spinner = ui.NewSpinner(fmt.Sprintf("Restarting %s", serviceName))

					if err := orch.Restart(ctx, serviceName); err != nil {
						spinner.Warning(fmt.Sprintf("Could not restart %s", serviceName))
					} else {
						spinner.Success(fmt.Sprintf("%s restarted", serviceName))
					}
				}

				ui.Blank()
				ui.Info("Note: If you added models in xatu-cbt, use 'xcli lab rebuild xatu-cbt' for full workflow")

			case "lab-backend":
				spinner := ui.NewSpinner("Rebuilding lab-backend")

				if err := orch.Builder().BuildLabBackend(ctx, true); err != nil {
					spinner.Fail("Failed to rebuild lab-backend")

					return fmt.Errorf("failed to rebuild lab-backend: %w", err)
				}

				spinner.Success("lab-backend rebuilt successfully")

				// Restart lab-backend
				spinner = ui.NewSpinner("Restarting lab-backend")

				if err := orch.Restart(ctx, "lab-backend"); err != nil {
					spinner.Fail("Could not restart lab-backend")
					ui.Info("If lab-backend is running, restart it manually:")
					ui.Info("  xcli lab restart lab-backend")
				} else {
					spinner.Success("lab-backend restarted successfully")
				}

			case "lab-frontend":
				spinner := ui.NewSpinner("Regenerating lab-frontend API types from cbt-api")

				if err := orch.Builder().BuildLabFrontend(ctx); err != nil {
					spinner.Fail("Failed to regenerate lab-frontend types")

					return fmt.Errorf("failed to regenerate lab-frontend types: %w", err)
				}

				spinner.Success("lab-frontend API types regenerated successfully")

				// Restart lab-frontend to apply changes
				spinner = ui.NewSpinner("Restarting lab-frontend")

				if err := orch.Restart(ctx, "lab-frontend"); err != nil {
					// If restart fails (e.g., service not running), just warn the user
					spinner.Fail("Could not restart lab-frontend")
					ui.Info("If lab-frontend is running, restart it manually:")
					ui.Info("  xcli lab restart lab-frontend")
				} else {
					spinner.Success("lab-frontend restarted successfully")
				}

			default:
				return fmt.Errorf("unknown project: %s\n\nSupported projects: xatu-cbt, cbt, cbt-api, lab-backend, lab-frontend, all", project)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose build output")

	return cmd
}

// runFullRebuild executes the full rebuild and restart workflow.
func runFullRebuild(
	ctx context.Context,
	log logrus.FieldLogger,
	orch *orchestrator.Orchestrator,
	verbose bool,
	stateDir string,
) error {
	// Create diagnostic report and store
	report := diagnostic.NewRebuildReport()
	store := diagnostic.NewStore(log, filepath.Join(stateDir, "errors"))

	ui.Header("Starting full rebuild and restart workflow")
	fmt.Println("This will:")
	fmt.Println("  - Regenerate all protos (xatu-cbt, cbt-api)")
	fmt.Println("  - Rebuild all binaries (xatu-cbt, cbt, cbt-api, lab-backend)")
	fmt.Println("  - Regenerate configs")
	fmt.Println("  - Restart all services")
	fmt.Println("  - Regenerate lab-frontend types")
	ui.Blank()

	// Track failures to skip dependent steps
	protoGenFailed := false
	xatuCBTBuildFailed := false
	cbtAPIFailed := false

	// Step 1: Regenerate xatu-cbt protos (MUST be first - everything depends on these)
	spinner := ui.NewSpinner("[1/7] Regenerating xatu-cbt protos")

	result := orch.Builder().GenerateXatuCBTProtosWithResult(ctx)
	report.AddResult(*result)

	if !result.Success {
		spinner.Fail("Failed to regenerate xatu-cbt protos")

		protoGenFailed = true
	} else {
		spinner.Success("xatu-cbt protos regenerated")
	}

	// Step 2: Rebuild xatu-cbt binary (skip if proto gen failed)
	if protoGenFailed {
		// Add skipped result
		now := time.Now()
		report.AddResult(diagnostic.BuildResult{
			Phase:     diagnostic.PhaseBuild,
			Service:   "xatu-cbt",
			Success:   false,
			ErrorMsg:  "skipped due to proto generation failure",
			StartTime: now,
			EndTime:   now,
		})

		xatuCBTBuildFailed = true
	} else {
		spinner = ui.NewSpinner("[2/7] Rebuilding xatu-cbt")

		result = orch.Builder().BuildXatuCBTWithResult(ctx, true)
		report.AddResult(*result)

		if !result.Success {
			spinner.Fail("Failed to rebuild xatu-cbt")

			xatuCBTBuildFailed = true
		} else {
			spinner.Success("xatu-cbt rebuilt")
		}
	}

	// Step 3: Regenerate cbt-api protos + rebuild cbt-api (depends on xatu-cbt)
	if xatuCBTBuildFailed {
		// Add skipped result for cbt-api
		now := time.Now()
		report.AddResult(diagnostic.BuildResult{
			Phase:     diagnostic.PhaseBuild,
			Service:   "cbt-api",
			Success:   false,
			ErrorMsg:  "skipped due to xatu-cbt build failure",
			StartTime: now,
			EndTime:   now,
		})

		cbtAPIFailed = true
	} else {
		spinner = ui.NewSpinner("[3/7] Regenerating cbt-api protos and rebuilding cbt-api")

		result = orch.Builder().BuildCBTAPIWithResult(ctx, true)
		report.AddResult(*result)

		if !result.Success {
			spinner.Fail("Failed to rebuild cbt-api")

			cbtAPIFailed = true
		} else {
			spinner.Success("cbt-api protos regenerated and cbt-api rebuilt")
		}
	}

	// Step 4: Rebuild remaining binaries (cbt, lab-backend)
	// These can proceed even if cbt-api failed
	spinner = ui.NewSpinner("[4/7] Rebuilding remaining binaries (cbt, lab-backend)")

	// Build CBT
	result = orch.Builder().BuildCBTWithResult(ctx, true)
	report.AddResult(*result)

	cbtFailed := !result.Success

	// Build lab-backend
	result = orch.Builder().BuildLabBackendWithResult(ctx, true)
	report.AddResult(*result)

	labBackendFailed := !result.Success

	if cbtFailed && labBackendFailed {
		spinner.Fail("Failed to rebuild cbt and lab-backend")
	} else if cbtFailed {
		spinner.Fail("Failed to rebuild cbt")
	} else if labBackendFailed {
		spinner.Fail("Failed to rebuild lab-backend")
	} else {
		spinner.Success("Remaining binaries rebuilt (cbt, lab-backend)")
	}

	// Step 5: Regenerate configs
	spinner = ui.NewSpinner("[5/7] Regenerating configs")

	configStart := time.Now()
	configErr := orch.GenerateConfigs()
	configEnd := time.Now()

	configResult := diagnostic.BuildResult{
		Phase:     diagnostic.PhaseConfigGen,
		Service:   "configs",
		Success:   configErr == nil,
		StartTime: configStart,
		EndTime:   configEnd,
		Duration:  configEnd.Sub(configStart),
	}
	if configErr != nil {
		configResult.Error = configErr
		configResult.ErrorMsg = configErr.Error()
	}

	report.AddResult(configResult)

	if configErr != nil {
		spinner.Fail("Failed to regenerate configs")
	} else {
		spinner.Success("Configs regenerated")
	}

	// Step 6: Restart ALL services (cbt-api + CBT engines + lab-backend)
	spinner = ui.NewSpinner("[6/7] Restarting all services (cbt-api + CBT engines + lab-backend)")

	// Check if services are running before attempting restart
	if !orch.AreServicesRunning() {
		_ = spinner.Stop()

		// Add skipped result for restart
		now := time.Now()
		report.AddResult(diagnostic.BuildResult{
			Phase:     diagnostic.PhaseRestart,
			Service:   "all-services",
			Success:   true, // Not a failure, just skipped
			ErrorMsg:  "services not running - skipped",
			StartTime: now,
			EndTime:   now,
		})

		// Finalize and display report
		report.Finalize()
		ui.Blank()
		ui.DisplayBuildSummary(report)

		// Save report
		if err := store.Save(report); err != nil {
			log.WithError(err).Warn("Failed to save diagnostic report")
		}

		if report.HasFailures() {
			ui.Blank()
			ui.Info("Run 'xcli lab diagnose' for error analysis")
			ui.Info("Run 'xcli lab diagnose --ai' for AI-powered diagnosis")

			return fmt.Errorf("rebuild failed: %d of %d steps failed",
				report.FailedCount, report.TotalCount)
		}

		ui.Blank()
		ui.Warning("Services not currently running - skipping restart and lab-frontend regeneration")
		ui.Info("All binaries, protos and configs have been regenerated.")
		ui.Info("Start services with: xcli lab up")

		return nil
	}

	restartStart := time.Now()
	restartErr := orch.RestartAllServices(ctx, verbose)
	restartEnd := time.Now()

	restartResult := diagnostic.BuildResult{
		Phase:     diagnostic.PhaseRestart,
		Service:   "all-services",
		Success:   restartErr == nil,
		StartTime: restartStart,
		EndTime:   restartEnd,
		Duration:  restartEnd.Sub(restartStart),
	}
	if restartErr != nil {
		restartResult.Error = restartErr
		restartResult.ErrorMsg = restartErr.Error()
	}

	report.AddResult(restartResult)

	if restartErr != nil {
		spinner.Fail("Failed to restart services")
	} else {
		spinner.Success("All services restarted")
	}

	// Step 7: Regenerate lab-frontend types (must be done after cbt-api is restarted)
	// Skip if restart failed or cbt-api failed
	if restartErr != nil || cbtAPIFailed {
		now := time.Now()
		skipReason := "skipped due to restart failure"

		if cbtAPIFailed {
			skipReason = "skipped due to cbt-api build failure"
		}

		report.AddResult(diagnostic.BuildResult{
			Phase:     diagnostic.PhaseFrontendGen,
			Service:   "lab-frontend",
			Success:   false,
			ErrorMsg:  skipReason,
			StartTime: now,
			EndTime:   now,
		})
	} else {
		spinner = ui.NewSpinner("[7/7] Regenerating lab-frontend API types")

		// Wait for cbt-api to be ready (it was just restarted)
		spinner.UpdateText("[7/7] Waiting for cbt-api to be ready")

		waitStart := time.Now()

		waitErr := orch.WaitForCBTAPIReady(ctx)
		if waitErr != nil {
			spinner.Fail("cbt-api did not become ready")

			waitEnd := time.Now()
			report.AddResult(diagnostic.BuildResult{
				Phase:     diagnostic.PhaseFrontendGen,
				Service:   "lab-frontend",
				Success:   false,
				Error:     waitErr,
				ErrorMsg:  fmt.Sprintf("cbt-api did not become ready: %v", waitErr),
				StartTime: waitStart,
				EndTime:   waitEnd,
				Duration:  waitEnd.Sub(waitStart),
			})
		} else {
			spinner.UpdateText("[7/7] Regenerating lab-frontend API types")

			result = orch.Builder().BuildLabFrontendWithResult(ctx)
			report.AddResult(*result)

			if !result.Success {
				spinner.Fail("Failed to regenerate lab-frontend types")
			} else {
				spinner.UpdateText("[7/7] Restarting lab-frontend")

				// Restart lab-frontend to apply changes
				if err := orch.Restart(ctx, "lab-frontend"); err != nil {
					spinner.Fail("Could not restart lab-frontend")
					ui.Warning(fmt.Sprintf("Could not restart lab-frontend: %v", err))
					ui.Info("If lab-frontend is running, restart it manually:")
					ui.Info("  xcli lab restart lab-frontend")
				} else {
					spinner.Success("Lab-frontend API types regenerated and service restarted")
				}
			}
		}
	}

	// Finalize and display report
	report.Finalize()
	ui.Blank()
	ui.DisplayBuildSummary(report)

	// Save report
	if err := store.Save(report); err != nil {
		log.WithError(err).Warn("Failed to save diagnostic report")
	}

	// Final status
	if report.HasFailures() {
		ui.Blank()
		ui.Info("Run 'xcli lab diagnose' for error analysis")
		ui.Info("Run 'xcli lab diagnose --ai' for AI-powered diagnosis")

		return fmt.Errorf("rebuild failed: %d of %d steps failed",
			report.FailedCount, report.TotalCount)
	}

	ui.Blank()
	ui.Success("Full rebuild and restart complete")

	return nil
}
