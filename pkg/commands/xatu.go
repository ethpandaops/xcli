package commands

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuCommand creates the xatu command namespace.
func NewXatuCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "xatu",
		Short: "Manage the xatu docker-compose stack",
		Long: `Manage the xatu docker-compose stack for local development.

The xatu stack is entirely docker-compose based, running ~25 services
including ClickHouse, Kafka, Grafana, and the various xatu components.

Common workflows:
  1. Initial setup:
     xcli xatu init           # Discover xatu repo, verify Docker
     xcli xatu check          # Verify environment is ready
     xcli xatu up             # Start the stack

  2. Development:
     (make code changes)
     xcli xatu rebuild xatu-server   # Rebuild and restart a service
     xcli xatu status                # Check service status
     xcli xatu logs xatu-server -f   # Stream logs

  3. Service control:
     xcli xatu stop <service>     # Stop a specific service
     xcli xatu start <service>    # Start a specific service
     xcli xatu restart <service>  # Restart a specific service

  4. Teardown:
     xcli xatu down              # Stop all containers
     xcli xatu clean             # Remove everything including volumes and images

Use 'xcli xatu [command] --help' for more information about a command.`,
	}

	// Add xatu subcommands
	cmd.AddCommand(NewXatuInitCommand(log, configPath))
	cmd.AddCommand(NewXatuCheckCommand(log, configPath))
	cmd.AddCommand(NewXatuUpCommand(log, configPath))
	cmd.AddCommand(NewXatuDownCommand(log, configPath))
	cmd.AddCommand(NewXatuCleanCommand(log, configPath))
	cmd.AddCommand(NewXatuStatusCommand(log, configPath))
	cmd.AddCommand(NewXatuStartCommand(log, configPath))
	cmd.AddCommand(NewXatuStopCommand(log, configPath))
	cmd.AddCommand(NewXatuRestartCommand(log, configPath))
	cmd.AddCommand(NewXatuLogsCommand(log, configPath))
	cmd.AddCommand(NewXatuBuildCommand(log, configPath))
	cmd.AddCommand(NewXatuRebuildCommand(log, configPath))

	return cmd
}
