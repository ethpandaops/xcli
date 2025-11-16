package commands

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabUpCommand creates the lab up command.
func NewLabUpCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var (
		mode    string
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

This command always rebuilds all projects to ensure everything is up to date.

Flags:
  --verbose   Enable verbose output for all operations
  --mode      Override mode for this run (local or hybrid)

Examples:
  xcli lab up              # Start all services (always rebuilds)
  xcli lab up --verbose    # Startup with detailed output`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config
			labCfg, cfgPath, err := config.LoadLabConfig(configPath)
			if err != nil {
				return err
			}

			// Override mode if specified
			if mode != "" {
				labCfg.Mode = mode
			}

			// Validate lab config
			if validationErr := labCfg.Validate(); validationErr != nil {
				return fmt.Errorf("invalid lab configuration: %w", validationErr)
			}

			// Create orchestrator
			orch, err := orchestrator.NewOrchestrator(log, labCfg, cfgPath)
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
				ui.Warning("Interrupt received, shutting down gracefully...")
				ui.Info("(Press Ctrl+C again to force quit)")

				// Stop all services
				if err := orch.StopServices(); err != nil {
					log.WithError(err).Error("failed to stop services gracefully")
					os.Exit(1)
				}

				ui.Success("All services stopped")
				os.Exit(0)
			}()

			// Always rebuild (skipBuild=false, forceBuild=true)
			return orch.Up(cmd.Context(), false, true)
		},
	}

	cmd.Flags().StringVarP(&mode, "mode", "m", "", "Override mode (local or hybrid)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show all build/setup command output (default: errors only)")

	return cmd
}
