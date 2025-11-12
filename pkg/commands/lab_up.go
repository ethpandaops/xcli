package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabUpCommand creates the lab up command
func NewLabUpCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var mode string
	var noBuild bool
	var rebuild bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start the lab stack",
		Long:  `Start infrastructure and all services in the lab development stack.

By default, this command will automatically build any missing binaries. Use flags to control build behavior.`,
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
			if err := cfg.Lab.Validate(); err != nil {
				return fmt.Errorf("invalid lab configuration: %w", err)
			}

			// Create orchestrator
			orch := orchestrator.NewOrchestrator(log, cfg.Lab)

			// Set verbose mode
			orch.SetVerbose(verbose)

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
