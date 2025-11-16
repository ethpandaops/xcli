package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	stackAll = "all"
	stackLab = "lab"
)

// displayConfigForStack marshals and displays config for specified stack(s).
// stack can be "all", "lab", or future stack names.
func displayConfigForStack(cfg *config.Config, stack string) error {
	var data []byte

	var err error

	switch stack {
	case stackAll:
		data, err = yaml.Marshal(cfg)
	case stackLab:
		if cfg.Lab == nil {
			return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
		}

		data, err = yaml.Marshal(cfg.Lab)
	// Future stacks:
	// case "contributoor":
	//     if cfg.Contributoor == nil {
	//         return fmt.Errorf("contributoor configuration not found")
	//     }
	//     data, err = yaml.Marshal(cfg.Contributoor)
	default:
		return fmt.Errorf("unknown stack: %s (supported: all, lab)", stack)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	fmt.Println(string(data))

	return nil
}

// validateConfigForStack validates and displays summary for specified stack(s).
func validateConfigForStack(cfg *config.Config, stack string) error {
	switch stack {
	case stackAll:
		if err := cfg.Validate(); err != nil {
			fmt.Printf("✗ Configuration is invalid:\n  %v\n", err)

			return err
		}

		fmt.Println("✓ Configuration is valid")

		// Show summary for each configured stack
		if cfg.Lab != nil {
			displayLabSummary(cfg.Lab)
		}
		// Future: if cfg.Contributoor != nil { displayContributoorSummary() }

	case stackLab:
		if cfg.Lab == nil {
			return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
		}

		if err := cfg.Lab.Validate(); err != nil {
			fmt.Printf("✗ Lab configuration is invalid:\n  %v\n", err)

			return err
		}

		fmt.Println("✓ Lab configuration is valid")
		displayLabSummary(cfg.Lab)

	default:
		return fmt.Errorf("unknown stack: %s (supported: all, lab)", stack)
	}

	return nil
}

// displayLabSummary shows a summary of lab configuration.
func displayLabSummary(labCfg *config.LabConfig) {
	fmt.Printf("\nLab Stack:\n")
	fmt.Printf("  Mode: %s\n", labCfg.Mode)
	fmt.Printf("  Networks: ")

	for i, net := range labCfg.EnabledNetworks() {
		if i > 0 {
			fmt.Print(", ")
		}

		fmt.Print(net.Name)
	}

	fmt.Println()
}

// NewConfigCommand creates the config command (global - shows all stacks by default).
func NewConfigCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var stackFilter string

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  `View and validate configuration for all stacks or specific stack.`,
	}

	// Add --stack flag to parent command for inheritance
	cmd.PersistentFlags().StringVar(&stackFilter, "stack", stackAll, "Filter by stack (all, lab)")

	// config show subcommand
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Long: `Show current configuration for all stacks or filtered by --stack flag.

Examples:
  xcli config show              # Show all stacks (default)
  xcli config show --stack=lab  # Show only lab stack
  xcli config show --stack=all  # Show all stacks (explicit)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			return displayConfigForStack(result.Config, stackFilter)
		},
	}

	// config validate subcommand
	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration",
		Long: `Validate configuration for all stacks or filtered by --stack flag.

Examples:
  xcli config validate              # Validate all stacks (default)
  xcli config validate --stack=lab  # Validate only lab stack
  xcli config validate --stack=all  # Validate all stacks (explicit)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			return validateConfigForStack(result.Config, stackFilter)
		},
	}

	cmd.AddCommand(showCmd)
	cmd.AddCommand(validateCmd)

	return cmd
}
