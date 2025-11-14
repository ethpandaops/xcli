package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewLabConfigCommand creates the lab config command.
func NewLabConfigCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage lab configuration",
		Long:  `View and validate lab stack configuration.`,
	}

	// config show subcommand
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current lab configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			data, err := yaml.Marshal(cfg.Lab)
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}

			fmt.Println(string(data))

			return nil
		},
	})

	// config validate subcommand
	cmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate lab configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			if err := cfg.Lab.Validate(); err != nil {
				fmt.Printf("✗ Lab configuration is invalid:\n  %v\n", err)

				return err
			}

			fmt.Println("✓ Lab configuration is valid")
			fmt.Printf("\nMode: %s\n", cfg.Lab.Mode)
			fmt.Printf("Networks: ")

			for i, net := range cfg.Lab.EnabledNetworks() {
				if i > 0 {
					fmt.Print(", ")
				}

				fmt.Print(net.Name)
			}

			fmt.Println()

			return nil
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

			fmt.Println("Regenerating service configurations...")

			if err := orch.GenerateConfigs(); err != nil {
				return fmt.Errorf("failed to regenerate configs: %w", err)
			}

			fmt.Println("\n✓ All service configurations regenerated successfully")
			fmt.Println("\nTo apply changes, restart affected services:")
			fmt.Println("  xcli lab restart lab-backend")
			fmt.Println("  xcli lab restart cbt-api-mainnet")
			fmt.Println("  xcli lab restart cbt-mainnet")

			return nil
		},
	})

	return cmd
}
