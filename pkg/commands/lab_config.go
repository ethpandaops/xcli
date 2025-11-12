package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
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

	return cmd
}
