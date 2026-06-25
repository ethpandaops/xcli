package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/cc"
	"github.com/ethpandaops/xcli/pkg/instance"
	"github.com/ethpandaops/xcli/pkg/workspace"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewCCCommand creates the Command Center web dashboard command.
func NewCCCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var (
		port             int
		instanceOverride string
		noOpen           bool
	)

	cmd := &cobra.Command{
		Use:   "cc",
		Short: "Launch the Command Center web dashboard",
		Long: `Launch a local web dashboard for monitoring and controlling the lab stack.

The Command Center provides:
  • Real-time service status and health monitoring
  • Live log streaming from all services
  • Interactive service controls (start/stop/restart/rebuild)
  • Infrastructure and git status overview
  • Configuration viewer

The dashboard opens automatically in your default browser.
Use --no-open to prevent this behavior.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, ws, err := workspace.LoadConfig(configPath, true, true)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			workspace.ResolveLabRepoPaths(cfg.Lab, ws.RootDir)

			var runtime *instance.Runtime
			if cfg.Lab != nil {
				runtime, err = instance.ResolveRuntimeFromWorkspace(cmd.Context(), ws, cfg.Lab, instanceOverride, instance.RuntimeOptions{
					ClaimPorts: false,
					ProbePorts: true,
				})
				if err != nil {
					return fmt.Errorf("failed to resolve lab instance runtime: %w", err)
				}

				if runtime.Workspace != nil && runtime.Workspace.ConfigPath != "" {
					cfg, ws, err = workspace.LoadConfig(runtime.Workspace.ConfigPath, true, false)
					if err != nil {
						return fmt.Errorf("failed to load selected instance config: %w", err)
					}

					workspace.ResolveLabRepoPaths(cfg.Lab, ws.RootDir)
					cfg.Lab = runtime.LabConfig
				}

				if !cmd.Flags().Changed("port") {
					port = runtime.Ports.CommandCenter
				}
			}

			if port == 0 {
				port = instance.DefaultCommandCenterPort
			}

			srv, err := cc.NewServerWithRuntime(log, cfg, ws.ConfigPath, port, runtime)
			if err != nil {
				return fmt.Errorf("failed to create server: %w", err)
			}

			return srv.Start(cmd.Context(), !noOpen)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 0, "Port for the web dashboard (default: selected instance port)")
	cmd.Flags().StringVar(&instanceOverride, "instance", "", "Lab instance id override")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Don't open browser automatically")

	return cmd
}
