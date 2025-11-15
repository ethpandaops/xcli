package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewConfigCommand creates the config command (global - shows all stacks).
func NewConfigCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration (all stacks)",
		Long:  `View and validate configuration for all stacks.`,
	}

	// config show subcommand
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current configuration (all stacks)",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			data, err := yaml.Marshal(result.Config)
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
		Short: "Validate configuration (all stacks)",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := result.Config.Validate(); err != nil {
				fmt.Printf("✗ Configuration is invalid:\n  %v\n", err)

				return err
			}

			fmt.Println("✓ Configuration is valid")

			// Show summary for each stack
			if result.Config.Lab != nil {
				fmt.Printf("\nLab Stack:\n")
				fmt.Printf("  Mode: %s\n", result.Config.Lab.Mode)
				fmt.Printf("  Networks: ")

				for i, net := range result.Config.Lab.EnabledNetworks() {
					if i > 0 {
						fmt.Print(", ")
					}

					fmt.Print(net.Name)
				}

				fmt.Println()
			}

			// Future stacks can be added here
			// if cfg.Contributoor != nil { ... }
			// if cfg.Xatu != nil { ... }

			return nil
		},
	})

	return cmd
}
