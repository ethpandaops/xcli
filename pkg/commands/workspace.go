package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/ethpandaops/xcli/pkg/workspace"
)

func printCommandWorkspaceSelection(ws *workspace.Workspace) {
	ui.Info(fmt.Sprintf("Config: %s", ws.ConfigPath))
	ui.Info(fmt.Sprintf("Overrides: %s", ws.OverridesPath))
	ui.Info(fmt.Sprintf("State dir: %s", ws.StateDir))
}
