package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabConfigCommand creates the lab config command.
// NOTE: This is maintained for backward compatibility. Consider using:
//
//	xcli config show --stack=lab
//	xcli config validate --stack=lab
func NewLabConfigCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage lab configuration",
		Long: `View and validate lab stack configuration.

Note: These commands are also available via:
  xcli config show --stack=lab
  xcli config validate --stack=lab`,
	}

	// config show subcommand - now uses shared helper
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current lab configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			return displayConfigForStack(result.Config, "lab")
		},
	})

	// config validate subcommand
	cmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate lab configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			return validateConfigForStack(result.Config, "lab")
		},
	})

	// config regenerate subcommand
	cmd.AddCommand(&cobra.Command{
		Use:   "regenerate",
		Short: "Regenerate all service configuration files",
		Long: `Regenerate all service configuration files from current .xcli.yaml settings.

This is useful when you've changed settings in .xcli.yaml (like enabling/disabling
networks) and need to update service configs without rebuilding or restarting.

Regenerates configurations for:
  - lab-backend (network routing, rate limiting, etc.)
  - cbt-api (for each network)
  - cbt engines (for each network, including model overrides)

Note: This does NOT restart services. Use 'xcli lab restart <service>' to apply
the new configs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			labCfg, cfgPath, err := config.LoadLabConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Create orchestrator
			orch, err := orchestrator.NewOrchestrator(log, labCfg, cfgPath)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			ui.Info("Regenerating service configurations...")

			if err := orch.GenerateConfigs(cmd.Context()); err != nil {
				return fmt.Errorf("failed to regenerate configs: %w", err)
			}

			ui.Success("All service configurations regenerated successfully")
			ui.Header("To apply changes, restart affected services:")
			fmt.Println("  xcli lab restart lab-backend")
			fmt.Println("  xcli lab restart cbt-api-mainnet")
			fmt.Println("  xcli lab restart cbt-mainnet")

			return nil
		},
	})

	return cmd
}
