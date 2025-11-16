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

			// Create orchestrator
			orch, err := orchestrator.NewOrchestrator(log, labCfg, cfgPath)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			// Set verbose mode
			orch.SetVerbose(verbose)

			// Create builder
			ctx := context.Background()

			// Route to appropriate build
			switch project {
			case "xatu-cbt", "all":
				// Full rebuild and restart workflow
				// Flow: xatu-cbt protos → rebuild xatu-cbt → cbt-api protos → rebuild cbt-api → rebuild other binaries → configs → restart → frontend
				ui.Header("Starting full rebuild and restart workflow")
				fmt.Println("This will:")
				fmt.Println("  • Regenerate all protos (xatu-cbt, cbt-api)")
				fmt.Println("  • Rebuild all binaries (xatu-cbt, cbt, cbt-api, lab-backend)")
				fmt.Println("  • Regenerate configs")
				fmt.Println("  • Restart all services")
				fmt.Println("  • Regenerate lab-frontend types")
				ui.Blank()

				// Step 1: Regenerate xatu-cbt protos (MUST be first - everything depends on these)
				spinner := ui.NewSpinner("[1/6] Regenerating xatu-cbt protos")

				if err := orch.Builder().GenerateXatuCBTProtos(ctx); err != nil {
					spinner.Fail("Failed to regenerate xatu-cbt protos")

					return fmt.Errorf("failed to regenerate xatu-cbt protos: %w", err)
				}

				spinner.Success("xatu-cbt protos regenerated")

				// Step 2: Rebuild xatu-cbt binary (now that protos are fresh)
				spinner = ui.NewSpinner("[2/6] Rebuilding xatu-cbt")

				if err := orch.Builder().BuildXatuCBT(ctx, true); err != nil {
					spinner.Fail("Failed to rebuild xatu-cbt")

					return fmt.Errorf("failed to rebuild xatu-cbt: %w", err)
				}

				spinner.Success("xatu-cbt rebuilt")

				// Step 3: Regenerate cbt-api protos + rebuild cbt-api (depends on xatu-cbt database schemas)
				spinner = ui.NewSpinner("[3/6] Regenerating cbt-api protos and rebuilding cbt-api")

				if err := orch.Builder().BuildCBTAPI(ctx, true); err != nil {
					spinner.Fail("Failed to rebuild cbt-api")

					return fmt.Errorf("failed to rebuild cbt-api: %w", err)
				}

				spinner.Success("cbt-api protos regenerated and cbt-api rebuilt")

				// Step 4: Rebuild remaining binaries (cbt, lab-backend)
				spinner = ui.NewSpinner("[4/6] Rebuilding remaining binaries (cbt, lab-backend)")

				// Build CBT
				if err := orch.Builder().BuildCBT(ctx, true); err != nil {
					spinner.Fail("Failed to rebuild CBT")

					return fmt.Errorf("failed to rebuild CBT: %w", err)
				}

				// Build lab-backend
				if err := orch.Builder().BuildLabBackend(ctx, true); err != nil {
					spinner.Fail("Failed to rebuild lab-backend")

					return fmt.Errorf("failed to rebuild lab-backend: %w", err)
				}

				spinner.Success("Remaining binaries rebuilt (cbt, lab-backend)")

				// Step 5: Regenerate configs
				spinner = ui.NewSpinner("[5/6] Regenerating configs")

				if err := orch.GenerateConfigs(); err != nil {
					spinner.Fail("Failed to regenerate configs")

					return fmt.Errorf("failed to regenerate configs: %w", err)
				}

				spinner.Success("Configs regenerated")

				// Step 6: Restart ALL services (cbt-api + CBT engines + lab-backend)
				spinner = ui.NewSpinner("[6/6] Restarting all services (cbt-api + CBT engines + lab-backend)")

				// Check if services are running before attempting restart
				if !orch.AreServicesRunning() {
					_ = spinner.Stop()

					ui.Warning("Services not currently running - skipping restart and lab-frontend regeneration")
					ui.Info("All binaries, protos and configs have been regenerated.")
					ui.Info("Start services with: xcli lab up")
					ui.Success("Full rebuild complete (restart skipped)")

					return nil
				}

				if err := orch.RestartAllServices(ctx, verbose); err != nil {
					spinner.Fail("Failed to restart services")

					return fmt.Errorf("failed to restart services: %w", err)
				}

				spinner.Success("All services restarted")

				// Step 7: Regenerate lab-frontend types (must be done after cbt-api is restarted)
				spinner = ui.NewSpinner("[7/7] Regenerating lab-frontend API types")

				// Wait for cbt-api to be ready (it was just restarted)
				spinner.UpdateText("[7/7] Waiting for cbt-api to be ready")

				if err := orch.WaitForCBTAPIReady(ctx); err != nil {
					spinner.Fail("cbt-api did not become ready")

					return fmt.Errorf("cbt-api did not become ready: %w", err)
				}

				spinner.UpdateText("[7/7] Regenerating lab-frontend API types")

				if err := orch.Builder().BuildLabFrontend(ctx); err != nil {
					spinner.Fail("Failed to regenerate lab-frontend types")

					return fmt.Errorf("failed to regenerate lab-frontend types: %w", err)
				}

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

				ui.Blank()
				ui.Success("Full rebuild and restart complete")
				ui.Info("  1. xatu-cbt protos regenerated")
				ui.Info("  2. xatu-cbt binary rebuilt")
				ui.Info("  3. cbt-api protos regenerated and cbt-api rebuilt")
				ui.Info("  4. Remaining binaries rebuilt (cbt, lab-backend)")
				ui.Info("  5. Configs regenerated with new models")
				ui.Info("  6. All services restarted (cbt, cbt-api, lab-backend)")
				ui.Info("  7. Lab-frontend API types regenerated and service restarted")

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
