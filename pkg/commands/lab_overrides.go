package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/configtui"
	"github.com/ethpandaops/xcli/pkg/workspace"
	"github.com/spf13/cobra"
)

// NewLabOverridesCommand creates the lab overrides command.
func NewLabOverridesCommand(configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "overrides",
		Short: "Manage CBT model overrides interactively",
		Long: `Launch an interactive TUI to manage .cbt-overrides.yaml.

The TUI allows you to:
  - Enable/disable external models (from models/external/)
  - Enable/disable transformation models (from models/transformations/)
  - Set environment variables for backfill limits:
    - EXTERNAL_MODEL_MIN_TIMESTAMP: Consensus layer backfill timestamp
    - EXTERNAL_MODEL_MIN_BLOCK: Execution layer backfill block number

Changes are saved to .cbt-overrides.yaml. Run 'xcli lab config regenerate'
to apply changes to CBT configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			labCfg, ws, err := workspace.LoadLabConfig(configPath, true)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			printCommandWorkspaceSelection(ws)

			return configtui.Run(labCfg.Repos.XatuCBT, ws.OverridesPath)
		},
	}
}
