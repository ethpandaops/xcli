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
		Short: "Build all lab projects",
		Long: `Build all lab projects without starting services.

This is useful for:
  - Pre-building before 'xcli lab up --no-build'
  - CI/CD pipelines
  - Verifying builds without starting infrastructure

Projects built:
  - xatu-cbt (Phase 0)
  - CBT, lab-backend, lab (Phase 2, parallel)
  - Protos and cbt-api (Phase 5-6)

For rebuilding specific projects during development, use 'xcli lab rebuild'.

Examples:
  xcli lab build         # Build all projects
  xcli lab build --force # Force rebuild even if binaries exist`,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if result.Config.Lab == nil {
				return fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
			}

			// Only validate repo paths for build command - infrastructure config not needed
			if err := result.Config.Lab.ValidateRepos(); err != nil {
				return fmt.Errorf("invalid lab configuration: %w", err)
			}

			buildMgr := builder.NewManager(log, result.Config.Lab)

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
