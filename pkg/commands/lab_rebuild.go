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
  xatu-cbt    - Regenerate protos + rebuild cbt-api + regenerate configs + restart services
                (Use this when you add/modify models in xatu-cbt)
  cbt         - Rebuild CBT only
  cbt-api     - Regenerate protos + rebuild cbt-api
  lab-backend - Rebuild lab-backend only
  lab         - Rebuild lab frontend only
  all         - Rebuild all projects (CBT, lab-backend, lab)

Examples:
  xcli lab rebuild xatu-cbt         # Full model update workflow (proto → cbt-api → configs → restart)
  xcli lab rebuild cbt-api          # Regenerate protos + rebuild cbt-api only
  xcli lab rebuild cbt --verbose    # Rebuild CBT with verbose output
  xcli lab rebuild all              # Rebuild everything (parallel)

Note: 'xatu-cbt' rebuild includes service restarts. Other rebuilds do NOT restart services.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project := args[0]

			// Load config
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			// Create orchestrator
			orch, err := orchestrator.NewOrchestrator(log, cfg.Lab)
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
				// Flow: xatu-cbt protos → regenerate protos → rebuild cbt-api → regenerate configs → restart services
				fmt.Println("Starting xatu-cbt model update workflow...")
				fmt.Println("This will: regenerate protos → rebuild cbt-api → regenerate configs → restart services")

				// Step 1: Regenerate protos + rebuild cbt-api
				fmt.Println("\n[1/3] Regenerating protos and rebuilding cbt-api...")

				if err := orch.Builder().BuildCBTAPI(ctx, true); err != nil {
					return fmt.Errorf("failed to rebuild cbt-api: %w", err)
				}

				// Step 2: Regenerate configs
				fmt.Println("\n[2/3] Regenerating configs...")

				if err := orch.GenerateConfigs(); err != nil {
					return fmt.Errorf("failed to regenerate configs: %w", err)
				}

				// Step 3: Restart services (cbt-api + CBT engines)
				fmt.Println("\n[3/3] Restarting services (cbt-api + CBT engines)...")

				// Check if services are running before attempting restart
				if !orch.AreServicesRunning() {
					fmt.Println("⚠ Services not currently running - skipping restart")
					fmt.Println("Protos and configs have been regenerated.")
					fmt.Println("Start services with: xcli lab up")
					fmt.Println("\n✓ xatu-cbt model update complete (restart skipped)")

					return nil
				}

				if err := orch.RestartServices(ctx, verbose); err != nil {
					return fmt.Errorf("failed to restart services: %w", err)
				}

				fmt.Println("\n✓ xatu-cbt model update complete")
				fmt.Println("  - Protos regenerated from xatu-cbt")
				fmt.Println("  - cbt-api rebuilt with new protos")
				fmt.Println("  - Configs regenerated with new models")
				fmt.Println("  - Services restarted and running")

			case "cbt":
				fmt.Println("Rebuilding CBT...")

				if err := orch.Builder().BuildCBT(ctx, true); err != nil {
					return fmt.Errorf("failed to rebuild CBT: %w", err)
				}

				fmt.Println("✓ CBT rebuilt successfully")
				fmt.Println("Note: You may need to restart CBT services for changes to take effect")

			case "cbt-api":
				fmt.Println("Regenerating protos and rebuilding cbt-api...")

				if err := orch.Builder().BuildCBTAPI(ctx, true); err != nil {
					return fmt.Errorf("failed to rebuild cbt-api: %w", err)
				}

				fmt.Println("✓ cbt-api rebuilt successfully (protos regenerated)")
				fmt.Println("Note: If you added models in xatu-cbt, use 'xcli lab rebuild xatu-cbt' for full workflow")

			case "lab-backend":
				fmt.Println("Rebuilding lab-backend...")

				if err := orch.Builder().BuildLabBackend(ctx, true); err != nil {
					return fmt.Errorf("failed to rebuild lab-backend: %w", err)
				}

				fmt.Println("✓ lab-backend rebuilt successfully")

			case "lab":
				fmt.Println("⚠ lab frontend is not a compiled project")
				fmt.Println("The frontend runs directly with 'pnpm dev' and doesn't need rebuilding.")
				fmt.Println("If you've made changes to the lab frontend, just restart the service:")
				fmt.Println("  xcli lab restart lab-frontend")

				return nil

			case "all":
				fmt.Println("Rebuilding all projects (parallel)...")
				// Uses DAG from Plan 3 for parallel execution
				if err := orch.Builder().BuildAll(ctx, true); err != nil {
					return fmt.Errorf("failed to rebuild all: %w", err)
				}

				fmt.Println("✓ All projects rebuilt successfully")

			default:
				return fmt.Errorf("unknown project: %s\n\nSupported projects: xatu-cbt, cbt, cbt-api, lab-backend, lab, all", project)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose build output")

	return cmd
}
