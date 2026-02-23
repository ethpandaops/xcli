package stack

import (
	"context"

	"github.com/spf13/cobra"
)

// ValidArgsFunc matches the cobra ValidArgsFunction signature.
type ValidArgsFunc = func(
	cmd *cobra.Command, args []string, toComplete string,
) ([]string, cobra.ShellCompDirective)

// Stack defines the interface for managing a docker-compose based
// development stack. Each implementation provides the business logic
// for a specific stack (lab, xatu, etc.) while the command factories
// in commands.go handle the shared cobra wiring.
type Stack interface {
	// Name returns the human-readable name of the stack (e.g. "lab", "xatu").
	Name() string

	// Lifecycle operations.
	Init(ctx context.Context) error
	Check(ctx context.Context) error
	Up(ctx context.Context) error
	Down(ctx context.Context) error
	Clean(ctx context.Context) error
	Build(ctx context.Context, services []string) error
	Rebuild(ctx context.Context, target string) error

	// Service operations.
	Start(ctx context.Context, service string) error
	Stop(ctx context.Context, service string) error
	Restart(ctx context.Context, service string) error
	Logs(ctx context.Context, service string, follow bool) error
	PrintStatus(ctx context.Context) error

	// ConfigureCommand is called during command creation to let the stack
	// add custom flags, set Long descriptions, examples, etc.
	ConfigureCommand(name string, cmd *cobra.Command)

	// Completions.
	CompleteServices() ValidArgsFunc
	CompleteRebuildTargets() ValidArgsFunc
}
