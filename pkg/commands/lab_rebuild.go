package commands

import (
	"context"
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabRebuildCommand creates the lab rebuild command.
func NewLabRebuildCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "rebuild [project]",
		Short: "Rebuild specific project(s) for local development",
		Long: `Rebuild one or more projects without full stack restart.

Useful for local development when you've made changes and need to rebuild
specific components. Much faster than 'xcli lab down && xcli lab up'.

Supported projects:
  xatu-cbt     - Regenerate protos + rebuild cbt-api + regenerate configs + restart services + regenerate lab-frontend types
                 (Use this when you add/modify models in xatu-cbt)
  cbt          - Rebuild CBT + restart CBT services
  cbt-api      - Regenerate protos + rebuild cbt-api + restart cbt-api services
  lab-backend  - Rebuild lab-backend + restart lab-backend service
  lab-frontend - Regenerate frontend API types from cbt-api OpenAPI spec + restart lab-frontend
  all          - Rebuild all projects (CBT, lab-backend, lab)

Examples:
  xcli lab rebuild xatu-cbt         # Full model update workflow (proto → cbt-api → configs → restart)
  xcli lab rebuild cbt-api          # Regenerate protos + rebuild cbt-api + restart
  xcli lab rebuild cbt --verbose    # Rebuild CBT with verbose output + restart
  xcli lab rebuild all              # Rebuild everything (parallel)

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
				fmt.Println("Starting xatu-cbt model update workflow...")
				fmt.Println("This will: regenerate protos → rebuild cbt-api → regenerate configs → restart services → regenerate lab-frontend types")

				// Step 1: Regenerate protos + rebuild cbt-api
				fmt.Println("\n[1/4] Regenerating protos and rebuilding cbt-api...")

				if err := orch.Builder().BuildCBTAPI(ctx, true); err != nil {
					return fmt.Errorf("failed to rebuild cbt-api: %w", err)
				}

				// Step 2: Regenerate configs
				fmt.Println("\n[2/4] Regenerating configs...")

				if err := orch.GenerateConfigs(); err != nil {
					return fmt.Errorf("failed to regenerate configs: %w", err)
				}

				// Step 3: Restart services (cbt-api + CBT engines)
				fmt.Println("\n[3/4] Restarting services (cbt-api + CBT engines)...")

				// Check if services are running before attempting restart
				if !orch.AreServicesRunning() {
					fmt.Println("⚠ Services not currently running - skipping restart and lab-frontend regeneration")
					fmt.Println("Protos and configs have been regenerated.")
					fmt.Println("Start services with: xcli lab up")
					fmt.Println("\n✓ xatu-cbt model update complete (restart skipped)")

					return nil
				}

				if err := orch.RestartServices(ctx, verbose); err != nil {
					return fmt.Errorf("failed to restart services: %w", err)
				}

				// Step 4: Regenerate lab-frontend types (must be done after cbt-api is restarted)
				fmt.Println("\n[4/4] Regenerating lab-frontend API types and restarting...")

				// Wait for cbt-api to be ready (it was just restarted)
				fmt.Println("Waiting for cbt-api to be ready...")

				if err := orch.WaitForCBTAPIReady(ctx); err != nil {
					return fmt.Errorf("cbt-api did not become ready: %w", err)
				}

				if err := orch.Builder().BuildLabFrontend(ctx); err != nil {
					return fmt.Errorf("failed to regenerate lab-frontend types: %w", err)
				}

				// Restart lab-frontend to apply changes
				if err := orch.Restart(ctx, "lab-frontend"); err != nil {
					fmt.Printf("⚠ Could not restart lab-frontend: %v\n", err)
					fmt.Println("If lab-frontend is running, restart it manually:")
					fmt.Println("  xcli lab restart lab-frontend")
				} else {
					fmt.Println("✓ lab-frontend restarted")
				}

				fmt.Println("\n✓ xatu-cbt model update complete")
				fmt.Println("  - Protos regenerated from xatu-cbt")
				fmt.Println("  - cbt-api rebuilt with new protos")
				fmt.Println("  - Configs regenerated with new models")
				fmt.Println("  - Services restarted and running")
				fmt.Println("  - Lab-frontend API types regenerated and service restarted")

			case "cbt":
				fmt.Println("Rebuilding CBT...")

				if err := orch.Builder().BuildCBT(ctx, true); err != nil {
					return fmt.Errorf("failed to rebuild CBT: %w", err)
				}

				fmt.Println("✓ CBT rebuilt successfully")

				// Restart all CBT services
				fmt.Println("\nRestarting CBT services...")

				enabledNetworks := orch.Config().EnabledNetworks()
				for _, network := range enabledNetworks {
					serviceName := fmt.Sprintf("cbt-%s", network.Name)
					if err := orch.Restart(ctx, serviceName); err != nil {
						fmt.Printf("⚠ Could not restart %s: %v\n", serviceName, err)
					} else {
						fmt.Printf("✓ %s restarted\n", serviceName)
					}
				}

			case "cbt-api":
				fmt.Println("Regenerating protos and rebuilding cbt-api...")

				if err := orch.Builder().BuildCBTAPI(ctx, true); err != nil {
					return fmt.Errorf("failed to rebuild cbt-api: %w", err)
				}

				fmt.Println("✓ cbt-api rebuilt successfully")

				// Restart all cbt-api services
				fmt.Println("\nRestarting cbt-api services...")

				enabledNetworks := orch.Config().EnabledNetworks()
				for _, network := range enabledNetworks {
					serviceName := fmt.Sprintf("cbt-api-%s", network.Name)
					if err := orch.Restart(ctx, serviceName); err != nil {
						fmt.Printf("⚠ Could not restart %s: %v\n", serviceName, err)
					} else {
						fmt.Printf("✓ %s restarted\n", serviceName)
					}
				}

				fmt.Println("\nNote: If you added models in xatu-cbt, use 'xcli lab rebuild xatu-cbt' for full workflow")

			case "lab-backend":
				fmt.Println("Rebuilding lab-backend...")

				if err := orch.Builder().BuildLabBackend(ctx, true); err != nil {
					return fmt.Errorf("failed to rebuild lab-backend: %w", err)
				}

				fmt.Println("✓ lab-backend rebuilt successfully")

				// Restart lab-backend
				fmt.Println("\nRestarting lab-backend...")

				if err := orch.Restart(ctx, "lab-backend"); err != nil {
					fmt.Printf("⚠ Could not restart lab-backend: %v\n", err)
					fmt.Println("If lab-backend is running, restart it manually:")
					fmt.Println("  xcli lab restart lab-backend")
				} else {
					fmt.Println("✓ lab-backend restarted successfully")
				}

			case "lab-frontend":
				fmt.Println("Regenerating lab-frontend API types from cbt-api...")

				if err := orch.Builder().BuildLabFrontend(ctx); err != nil {
					return fmt.Errorf("failed to regenerate lab-frontend types: %w", err)
				}

				fmt.Println("✓ lab-frontend API types regenerated successfully")

				// Restart lab-frontend to apply changes
				fmt.Println("\nRestarting lab-frontend...")

				if err := orch.Restart(ctx, "lab-frontend"); err != nil {
					// If restart fails (e.g., service not running), just warn the user
					fmt.Printf("⚠ Could not restart lab-frontend: %v\n", err)
					fmt.Println("If lab-frontend is running, restart it manually:")
					fmt.Println("  xcli lab restart lab-frontend")
				} else {
					fmt.Println("✓ lab-frontend restarted successfully")
				}

			case "all":
				fmt.Println("Rebuilding all projects")
				// Uses DAG from Plan 3 for parallel execution
				if err := orch.Builder().BuildAll(ctx, true); err != nil {
					return fmt.Errorf("failed to rebuild all: %w", err)
				}

				fmt.Println("✓ All projects rebuilt successfully")

			default:
				return fmt.Errorf("unknown project: %s\n\nSupported projects: xatu-cbt, cbt, cbt-api, lab-backend, lab-frontend, all", project)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose build output")

	return cmd
}
