package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewConfigCommand creates the config command
func NewConfigCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  `View and validate configuration.`,
	}

	// config show subcommand
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			data, err := yaml.Marshal(cfg)
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
		Short: "Validate configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := cfg.Validate(); err != nil {
				fmt.Printf("✗ Configuration is invalid:\n  %v\n", err)
				return err
			}

			fmt.Println("✓ Configuration is valid")
			fmt.Printf("\nMode: %s\n", cfg.Mode)
			fmt.Printf("Networks: ")
			for i, net := range cfg.EnabledNetworks() {
				if i > 0 {
					fmt.Print(", ")
				}
				fmt.Print(net.Name)
			}
			fmt.Println()

			return nil
		},
	})

	return cmd
}
