package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/builder"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabBuildCommand creates the lab build command.
func NewLabBuildCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build all lab repositories",
		Long: `Build all required binaries for the lab stack.

This command builds:
- xatu-cbt (infrastructure tool)
- cbt (transformation engine)
- cbt-api (REST API server) - requires infrastructure to be running for proto generation
- lab-backend (API gateway)
- lab (installs frontend dependencies)

Note: This command does NOT start infrastructure or generate protos.
For a complete build including proto generation, use 'xcli lab up --rebuild'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			if err := cfg.Lab.Validate(); err != nil {
				return fmt.Errorf("invalid lab configuration: %w", err)
			}

			buildMgr := builder.NewManager(log, cfg.Lab)

			fmt.Println("building all lab repositories")

			if err := buildMgr.BuildAll(cmd.Context(), force); err != nil {
				return fmt.Errorf("build failed: %w", err)
			}

			fmt.Println("\n✓ Build complete!")
			fmt.Println("\nNote: cbt-api protos not generated (requires infrastructure).")
			fmt.Println("Run 'xcli lab up' to start infrastructure and complete the build.")

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force rebuild even if binaries exist")

	return cmd
}
