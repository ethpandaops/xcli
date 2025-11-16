package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/builder"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabBuildCommand creates the lab build command.
func NewLabBuildCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build all lab projects from source without starting services",
		Long: `Build all lab projects from source without starting services.

Purpose:
  This command is designed for CI/CD pipelines and pre-building scenarios.
  It compiles all binaries but does NOT start any infrastructure or services.

Use cases:
  • Pre-building before starting services
  • CI/CD build verification pipelines
  • Checking for compilation errors without running services
  • Creating clean builds from scratch

What gets built:
  Phase 1: xatu-cbt (proto definitions)
  Phase 2: CBT, lab-backend, lab-frontend (parallel)
  Phase 3: Proto generation + cbt-api (requires xatu-cbt protos)

This command always rebuilds all projects to ensure everything is up to date.

Note: This does NOT generate configs or start services. For active development
with running services, use 'xcli lab rebuild' instead.

Key difference from 'rebuild':
  • build  = Build everything, don't start services (CI/CD)
  • rebuild = Build specific component + restart its services (development)

Examples:
  xcli lab build         # Build all projects`,
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

			ui.Header("Building all lab repositories")
			ui.Blank()

			// Always force rebuild
			if err := buildMgr.BuildAll(cmd.Context(), true); err != nil {
				return fmt.Errorf("build failed: %w", err)
			}

			ui.Success("Build complete!")
			ui.Info("Note: cbt-api protos not generated (requires infrastructure).")
			ui.Info("Run 'xcli lab up' to start infrastructure and complete the build.")

			return nil
		},
	}

	return cmd
}
