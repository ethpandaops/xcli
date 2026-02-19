package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/ui"
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
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeModes(),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := args[0]
			if mode != constants.ModeLocal && mode != constants.ModeHybrid {
				return fmt.Errorf("invalid mode: %s (must be '%s' or '%s')", mode, constants.ModeLocal, constants.ModeHybrid)
			}

			// Load config (need full config to save it later)
			result, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if result.Config.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			labCfg := result.Config.Lab
			cfgPath := result.ConfigPath

			// Check if already in requested mode
			if labCfg.Mode == mode {
				ui.Info(fmt.Sprintf("%s mode already active", mode))

				return nil
			}

			// Update mode and ClickHouse mode
			// Note: We only change the mode fields, preserving any external credentials
			// so users can switch back and forth without losing their config
			oldMode := labCfg.Mode
			labCfg.Mode = mode

			hasExternalCredentials := true

			if mode == constants.ModeHybrid {
				labCfg.Infrastructure.ClickHouse.Xatu.Mode = constants.InfraModeExternal

				// Check if external credentials are configured
				if labCfg.Infrastructure.ClickHouse.Xatu.ExternalURL == "" {
					hasExternalCredentials = false

					ui.Warning("Switching to hybrid mode but no external ClickHouse URL configured")
					ui.Info("You'll need to set this in .xcli.yaml before running 'xcli lab up':")
					fmt.Println("  lab:")
					fmt.Println("    infrastructure:")
					fmt.Println("      clickhouse:")
					fmt.Println("        xatu:")
					fmt.Println("          externalUrl: \"https://username:password@host:port\"")
					fmt.Println("          externalDatabase: \"default\"")
				}
			} else {
				labCfg.Infrastructure.ClickHouse.Xatu.Mode = constants.InfraModeLocal

				// Inform user that external credentials are preserved
				if oldMode == constants.ModeHybrid && labCfg.Infrastructure.ClickHouse.Xatu.ExternalURL != "" {
					ui.Success("External ClickHouse credentials preserved for future hybrid mode use")
				}
			}

			// Save config
			if err := result.Config.Save(cfgPath); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			log.WithField("mode", mode).Info("mode updated")
			ui.Success(fmt.Sprintf("Mode switched to: %s", mode))

			// Don't offer to restart if switching to hybrid without credentials
			if mode == constants.ModeHybrid && !hasExternalCredentials {
				ui.Info("Please configure external ClickHouse credentials in .xcli.yaml")
				ui.Info("Then run: xcli lab down && xcli lab up")

				return nil
			}

			ui.Header("Restart stack to apply changes:")
			fmt.Println("  xcli lab down && xcli lab up")

			// Optionally restart services automatically
			fmt.Print("Restart services now? (y/N): ")

			var response string

			_, _ = fmt.Scanln(&response)
			if response == "y" || response == "Y" {
				orch, err := orchestrator.NewOrchestrator(log, labCfg, cfgPath)
				if err != nil {
					return fmt.Errorf("failed to create orchestrator: %w", err)
				}
				// Tear down infrastructure completely
				// This is necessary because local vs hybrid mode use different infrastructure
				if err := orch.Down(cmd.Context(), nil); err != nil {
					return fmt.Errorf("failed to tear down: %w", err)
				}
				// Restart with auto-build enabled, no force rebuild
				if err := orch.Up(cmd.Context(), false, false, nil); err != nil {
					return fmt.Errorf("failed to start services: %w", err)
				}

				ui.Success("Services restarted in new mode")
			}

			return nil
		},
	}

	return cmd
}
