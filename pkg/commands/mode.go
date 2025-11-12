package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewModeCommand creates the mode command
func NewModeCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mode <local|hybrid>",
		Short: "Switch between local and hybrid mode",
		Long:  `Switch between local mode (all services local) and hybrid mode (external data).`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := args[0]
			if mode != "local" && mode != "hybrid" {
				return fmt.Errorf("invalid mode: %s (must be 'local' or 'hybrid')", mode)
			}

			// Load config
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Update mode
			cfg.Mode = mode
			if mode == "hybrid" {
				cfg.Infrastructure.ClickHouse.Xatu.Mode = "external"
			} else {
				cfg.Infrastructure.ClickHouse.Xatu.Mode = "local"
			}

			// Save config
			if err := cfg.Save(configPath); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			log.WithField("mode", mode).Info("Mode updated")
			fmt.Printf("\n✓ Mode switched to: %s\n", mode)
			fmt.Println("\nRestart services to apply changes:")
			fmt.Println("  xcli down && xcli up\n")

			// Optionally restart services automatically
			fmt.Print("Restart services now? (y/N): ")
			var response string
			fmt.Scanln(&response)
			if response == "y" || response == "Y" {
				orch := orchestrator.NewOrchestrator(log, cfg)
				if err := orch.Down(cmd.Context()); err != nil {
					return fmt.Errorf("failed to stop services: %w", err)
				}
				// Restart with auto-build enabled, no force rebuild
				if err := orch.Up(cmd.Context(), false, false); err != nil {
					return fmt.Errorf("failed to start services: %w", err)
				}
				fmt.Println("\n✓ Services restarted in new mode")
			}

			return nil
		},
	}

	return cmd
}
