package stack

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewInitCommand creates the init subcommand for a stack.
func NewInitCommand(s Stack) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: fmt.Sprintf("Initialize the %s environment", s.Name()),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.Init(cmd.Context())
		},
	}

	s.ConfigureCommand("init", cmd)

	return cmd
}

// NewCheckCommand creates the check subcommand for a stack.
func NewCheckCommand(s Stack) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: fmt.Sprintf("Verify %s environment is ready", s.Name()),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.Check(cmd.Context())
		},
	}

	s.ConfigureCommand("check", cmd)

	return cmd
}

// NewUpCommand creates the up subcommand for a stack.
func NewUpCommand(s Stack) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up",
		Short: fmt.Sprintf("Start the %s stack", s.Name()),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.Up(cmd.Context())
		},
	}

	s.ConfigureCommand("up", cmd)

	return cmd
}

// NewDownCommand creates the down subcommand for a stack.
func NewDownCommand(s Stack) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "down",
		Short: fmt.Sprintf("Stop the %s stack", s.Name()),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.Down(cmd.Context())
		},
	}

	s.ConfigureCommand("down", cmd)

	return cmd
}

// NewCleanCommand creates the clean subcommand for a stack.
func NewCleanCommand(s Stack) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: fmt.Sprintf("Remove all %s containers and artifacts", s.Name()),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.Clean(cmd.Context())
		},
	}

	s.ConfigureCommand("clean", cmd)

	return cmd
}

// NewBuildCommand creates the build subcommand for a stack.
func NewBuildCommand(s Stack) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: fmt.Sprintf("Build %s projects", s.Name()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.Build(cmd.Context(), args)
		},
	}

	s.ConfigureCommand("build", cmd)

	return cmd
}

// NewRebuildCommand creates the rebuild subcommand for a stack.
func NewRebuildCommand(s Stack) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "rebuild <target>",
		Short:             fmt.Sprintf("Rebuild and restart a %s component", s.Name()),
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: s.CompleteRebuildTargets(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.Rebuild(cmd.Context(), args[0])
		},
	}

	s.ConfigureCommand("rebuild", cmd)

	return cmd
}

// NewStatusCommand creates the status subcommand for a stack.
func NewStatusCommand(s Stack) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: fmt.Sprintf("Show %s stack status", s.Name()),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.PrintStatus(cmd.Context())
		},
	}

	s.ConfigureCommand("status", cmd)

	return cmd
}

// NewLogsCommand creates the logs subcommand for a stack.
func NewLogsCommand(s Stack) *cobra.Command {
	var follow bool

	cmd := &cobra.Command{
		Use:               "logs [service]",
		Short:             fmt.Sprintf("Show %s service logs", s.Name()),
		ValidArgsFunction: s.CompleteServices(),
		RunE: func(cmd *cobra.Command, args []string) error {
			service := ""
			if len(args) > 0 {
				service = args[0]
			}

			return s.Logs(cmd.Context(), service, follow)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	s.ConfigureCommand("logs", cmd)

	return cmd
}

// NewStartCommand creates the start subcommand for a stack.
func NewStartCommand(s Stack) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "start <service>",
		Short:             fmt.Sprintf("Start a specific %s service", s.Name()),
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: s.CompleteServices(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.Start(cmd.Context(), args[0])
		},
	}

	s.ConfigureCommand("start", cmd)

	return cmd
}

// NewStopCommand creates the stop subcommand for a stack.
func NewStopCommand(s Stack) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "stop <service>",
		Short:             fmt.Sprintf("Stop a specific %s service", s.Name()),
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: s.CompleteServices(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.Stop(cmd.Context(), args[0])
		},
	}

	s.ConfigureCommand("stop", cmd)

	return cmd
}

// NewRestartCommand creates the restart subcommand for a stack.
func NewRestartCommand(s Stack) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "restart <service>",
		Short:             fmt.Sprintf("Restart a specific %s service", s.Name()),
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: s.CompleteServices(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.Restart(cmd.Context(), args[0])
		},
	}

	s.ConfigureCommand("restart", cmd)

	return cmd
}
