package commands

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabUpCommand creates the lab up command.
func NewLabUpCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var (
		mode    string
		noBuild bool
		rebuild bool
		verbose bool
	)

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start the xcli lab stack",
		Long: `Start the complete xcli lab stack including infrastructure and services.

Prerequisites must be satisfied before running this command. If you haven't
already, run 'xcli lab init' first to ensure all prerequisites are met.

The stack starts in the configured mode (local or hybrid). Use 'xcli lab mode'
to switch between modes.

Flags:
  --no-build  Skip building projects (use existing builds)
  --rebuild   Force rebuild all projects from scratch
  --verbose   Enable verbose output for all operations
  --mode      Override mode for this run (local or hybrid)

Examples:
  xcli lab up              # Normal startup
  xcli lab up --verbose    # Startup with detailed output
  xcli lab up --rebuild    # Force rebuild everything`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Check lab config exists
			if cfg.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			// Override mode if specified
			if mode != "" {
				cfg.Lab.Mode = mode
			}

			// Validate lab config
			if validationErr := cfg.Lab.Validate(); validationErr != nil {
				return fmt.Errorf("invalid lab configuration: %w", validationErr)
			}

			// Create orchestrator
			orch, err := orchestrator.NewOrchestrator(log, cfg.Lab)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			// Set verbose mode
			orch.SetVerbose(verbose)

			// Setup signal handling for graceful shutdown
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			// Start signal handler in background
			go func() {
				sig := <-sigChan
				log.WithField("signal", sig.String()).Info("received shutdown signal")
				fmt.Println("\n\n⚠ Interrupt received, shutting down gracefully...")
				fmt.Println("(Press Ctrl+C again to force quit)")

				// Stop all services
				if err := orch.StopServices(); err != nil {
					log.WithError(err).Error("failed to stop services gracefully")
					os.Exit(1)
				}

				fmt.Println("✓ All services stopped")
				os.Exit(0)
			}()

			// Start everything
			return orch.Up(cmd.Context(), noBuild, rebuild)
		},
	}

	cmd.Flags().StringVarP(&mode, "mode", "m", "", "Override mode (local or hybrid)")
	cmd.Flags().BoolVar(&noBuild, "no-build", false, "Skip building, fail if binaries are missing")
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "Force rebuild all binaries even if they exist")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show all build/setup command output (default: errors only)")

	return cmd
}
