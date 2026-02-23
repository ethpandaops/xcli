package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewXatuStatusCommand creates the xatu status command.
func NewXatuStatusCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show xatu stack status",
		Long: `Display status of all xatu services.

Shows:
  - Running services and their states
  - Port bindings
  - Container health

Example:
  xcli xatu status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			xatuCfg, _, err := config.LoadXatuConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			runner, err := newXatuRunner(log, xatuCfg)
			if err != nil {
				return fmt.Errorf("failed to create compose runner: %w", err)
			}

			return printXatuStatus(cmd.Context(), runner)
		},
	}
}
