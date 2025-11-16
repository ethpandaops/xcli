package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabModeCommand creates the lab mode command.
func NewLabModeCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mode <local|hybrid>",
		Short: "Switch deployment mode between fully local and hybrid external data source",
		Long: `Switch deployment mode between fully local and hybrid external data source.

Modes:
  local  - Complete local development environment (default)
           • All services run locally in Docker
           • Local Xatu ClickHouse for data source
           • Local CBT ClickHouse for processing
           • No external dependencies required
           • Best for: Isolated development, testing, demos

  hybrid - Mixed local processing with external production data
           • External Xatu ClickHouse (production data source)
           • Local CBT ClickHouse (local processing and storage)
           • Local Redis for caching
           • Requires: external_clickhouse configuration in .xcli.yaml
           • Best for: Testing against production data, debugging live issues

After switching modes, restart the stack for changes to take effect:
  xcli lab down && xcli lab up

Examples:
  xcli lab mode local        # Switch to fully local mode
  xcli lab mode hybrid       # Switch to hybrid mode (requires external ClickHouse config)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := args[0]
			if mode != constants.ModeLocal && mode != constants.ModeHybrid {
				return fmt.Errorf("invalid mode: %s (must be '%s' or '%s')", mode, constants.ModeLocal, constants.ModeHybrid)
			}

			// Load config
			result, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if result.Config.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			// Update mode and ClickHouse mode
			// Note: We only change the mode fields, preserving any external credentials
			// so users can switch back and forth without losing their config
			oldMode := result.Config.Lab.Mode
			result.Config.Lab.Mode = mode

			hasExternalCredentials := true

			if mode == constants.ModeHybrid {
				result.Config.Lab.Infrastructure.ClickHouse.Xatu.Mode = constants.InfraModeExternal

				// Check if external credentials are configured
				if result.Config.Lab.Infrastructure.ClickHouse.Xatu.ExternalURL == "" {
					hasExternalCredentials = false

					fmt.Println("\n⚠ Warning: Switching to hybrid mode but no external ClickHouse URL configured")
					fmt.Println("You'll need to set this in .xcli.yaml before running 'xcli lab up':")
					fmt.Println("  lab:")
					fmt.Println("    infrastructure:")
					fmt.Println("      clickhouse:")
					fmt.Println("        xatu:")
					fmt.Println("          externalUrl: \"https://username:password@host:port\"")
					fmt.Println("          externalDatabase: \"default\"")
				}
			} else {
				result.Config.Lab.Infrastructure.ClickHouse.Xatu.Mode = constants.InfraModeLocal

				// Inform user that external credentials are preserved
				if oldMode == constants.ModeHybrid && result.Config.Lab.Infrastructure.ClickHouse.Xatu.ExternalURL != "" {
					fmt.Println("\n✓ External ClickHouse credentials preserved for future hybrid mode use")
				}
			}

			// Save config
			if err := result.Config.Save(result.ConfigPath); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			log.WithField("mode", mode).Info("mode updated")
			fmt.Printf("\n✓ Mode switched to: %s\n", mode)

			// Don't offer to restart if switching to hybrid without credentials
			if mode == constants.ModeHybrid && !hasExternalCredentials {
				fmt.Println("\nPlease configure external ClickHouse credentials in .xcli.yaml")
				fmt.Println("Then run: xcli lab down && xcli lab up")

				return nil
			}

			fmt.Println("\nRestart stack to apply changes (infrastructure will be rebuilt):")
			fmt.Println("  xcli lab down && xcli lab up")

			// Optionally restart services automatically
			fmt.Print("Restart services now? (y/N): ")

			var response string

			_, _ = fmt.Scanln(&response)
			if response == "y" || response == "Y" {
				orch, err := orchestrator.NewOrchestrator(log, result.Config.Lab, result.ConfigPath)
				if err != nil {
					return fmt.Errorf("failed to create orchestrator: %w", err)
				}
				// Tear down infrastructure completely
				// This is necessary because local vs hybrid mode use different infrastructure
				if err := orch.Down(cmd.Context()); err != nil {
					return fmt.Errorf("failed to tear down: %w", err)
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
