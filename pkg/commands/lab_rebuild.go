package commands

import (
	"context"
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
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
  xatu-cbt     - Full model update workflow
                 (protos → cbt-api → configs → restart → frontend types)
                 Use when: You add/modify models in xatu-cbt

  cbt          - Rebuild CBT binary + restart all CBT services
                 Use when: You modify CBT engine code

  cbt-api      - Regenerate protos + rebuild + restart all cbt-api services
                 Use when: You modify cbt-api endpoints

  lab-backend  - Rebuild + restart lab-backend service
                 Use when: You modify lab-backend code

  lab-frontend - Regenerate API types + restart lab-frontend
                 Use when: cbt-api OpenAPI spec changed

  all          - Rebuild everything in parallel + restart all services
                 Use when: Multiple changes across projects

Examples:
  xcli lab rebuild xatu-cbt          # Full model update (most common)
  xcli lab rebuild cbt               # Quick CBT engine iteration
  xcli lab rebuild lab-backend -v    # Rebuild with verbose output

Note: All rebuild commands automatically restart their respective services if running.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project := args[0]

			// Load config
			result, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if result.Config.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			// Create orchestrator
			orch, err := orchestrator.NewOrchestrator(log, result.Config.Lab, result.ConfigPath)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			// Set verbose mode
			orch.SetVerbose(verbose)

			// Create builder
			ctx := context.Background()

			// Route to appropriate build
			switch project {
			case "xatu-cbt":
				// Full xatu-cbt model update workflow
				// Flow: xatu-cbt protos → regenerate protos → rebuild cbt-api → regenerate configs → restart services → regenerate lab-frontend types
				ui.Header("Starting xatu-cbt model update workflow")
				ui.Info("This will: regenerate protos → rebuild cbt-api → regenerate configs → restart services → regenerate lab-frontend types")
				ui.Blank()

				// Step 1: Regenerate protos + rebuild cbt-api
				spinner := ui.NewSpinner("[1/4] Regenerating protos and rebuilding cbt-api")

				if err := orch.Builder().BuildCBTAPI(ctx, true); err != nil {
					spinner.Fail("Failed to rebuild cbt-api")

					return fmt.Errorf("failed to rebuild cbt-api: %w", err)
				}

				spinner.Success("Protos regenerated and cbt-api rebuilt")

				// Step 2: Regenerate configs
				spinner = ui.NewSpinner("[2/4] Regenerating configs")

				if err := orch.GenerateConfigs(); err != nil {
					spinner.Fail("Failed to regenerate configs")

					return fmt.Errorf("failed to regenerate configs: %w", err)
				}

				spinner.Success("Configs regenerated")

				// Step 3: Restart services (cbt-api + CBT engines)
				spinner = ui.NewSpinner("[3/4] Restarting services (cbt-api + CBT engines)")

				// Check if services are running before attempting restart
				if !orch.AreServicesRunning() {
					_ = spinner.Stop()

					ui.Warning("Services not currently running - skipping restart and lab-frontend regeneration")
					ui.Info("Protos and configs have been regenerated.")
					ui.Info("Start services with: xcli lab up")
					ui.Success("xatu-cbt model update complete (restart skipped)")

					return nil
				}

				if err := orch.RestartServices(ctx, verbose); err != nil {
					spinner.Fail("Failed to restart services")

					return fmt.Errorf("failed to restart services: %w", err)
				}

				spinner.Success("Services restarted")

				// Step 4: Regenerate lab-frontend types (must be done after cbt-api is restarted)
				spinner = ui.NewSpinner("[4/4] Regenerating lab-frontend API types")

				// Wait for cbt-api to be ready (it was just restarted)
				spinner.UpdateText("[4/4] Waiting for cbt-api to be ready")

				if err := orch.WaitForCBTAPIReady(ctx); err != nil {
					spinner.Fail("cbt-api did not become ready")

					return fmt.Errorf("cbt-api did not become ready: %w", err)
				}

				spinner.UpdateText("[4/4] Regenerating lab-frontend API types")

				if err := orch.Builder().BuildLabFrontend(ctx); err != nil {
					spinner.Fail("Failed to regenerate lab-frontend types")

					return fmt.Errorf("failed to regenerate lab-frontend types: %w", err)
				}

				spinner.UpdateText("[4/4] Restarting lab-frontend")

				// Restart lab-frontend to apply changes
				if err := orch.Restart(ctx, "lab-frontend"); err != nil {
					spinner.Fail("Could not restart lab-frontend")
					ui.Warning(fmt.Sprintf("Could not restart lab-frontend: %v", err))
					ui.Info("If lab-frontend is running, restart it manually:")
					ui.Info("  xcli lab restart lab-frontend")
				} else {
					spinner.Success("Lab-frontend API types regenerated and service restarted")
				}

				ui.Blank()
				ui.Success("xatu-cbt model update complete")
				ui.Info("  - Protos regenerated from xatu-cbt")
				ui.Info("  - cbt-api rebuilt with new protos")
				ui.Info("  - Configs regenerated with new models")
				ui.Info("  - Services restarted and running")
				ui.Info("  - Lab-frontend API types regenerated and service restarted")

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

			case "all":
				spinner := ui.NewSpinner("Rebuilding all projects")
				// Uses DAG from Plan 3 for parallel execution
				if err := orch.Builder().BuildAll(ctx, true); err != nil {
					spinner.Fail("Failed to rebuild all projects")

					return fmt.Errorf("failed to rebuild all: %w", err)
				}

				spinner.Success("All projects rebuilt successfully")

			default:
				return fmt.Errorf("unknown project: %s\n\nSupported projects: xatu-cbt, cbt, cbt-api, lab-backend, lab-frontend, all", project)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose build output")

	return cmd
}
