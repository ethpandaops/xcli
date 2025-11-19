package commands

import (
	"context"
	"errors"
	"fmt"

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

			// Always rebuild (skipBuild=false, forceBuild=true)
			err = orch.Up(cmd.Context(), false, true)

			// Handle cancellation gracefully
			if err != nil && errors.Is(err, context.Canceled) {
				ui.Warning("Interrupt received, shutting down gracefully...")

				// Use a new context for cleanup since the original is cancelled
				cleanupCtx := context.Background()

				// Stop all services that may have been started
				if stopErr := orch.StopServices(cleanupCtx); stopErr != nil {
					log.WithError(stopErr).Error("failed to stop services gracefully")
				} else {
					ui.Success("All services stopped")
				}

				return nil // Return nil so os.Exit(0) is used
			}

			return err
		},
	}

	cmd.Flags().StringVarP(&mode, "mode", "m", "", "Override mode (local or hybrid)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show all build/setup command output (default: errors only)")

	return cmd
}
