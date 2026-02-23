package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/ethpandaops/xcli/pkg/compose"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/pterm/pterm"
	"github.com/sirupsen/logrus"
)

// newXatuRunner creates a compose.Runner from xatu configuration.
func newXatuRunner(log logrus.FieldLogger, cfg *config.XatuConfig) (*compose.Runner, error) {
	return compose.NewRunner(log, cfg.Repos.Xatu, cfg.Profiles, cfg.EnvOverrides)
}

// printXatuStatus fetches and displays xatu service status as a formatted table.
func printXatuStatus(ctx context.Context, runner *compose.Runner) error {
	statuses, err := runner.PS(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service status: %w", err)
	}

	if len(statuses) == 0 {
		ui.Info("No services running")

		return nil
	}

	headers := []string{"Service", "State", "Status", "Ports"}
	rows := make([][]string, 0, len(statuses))

	for _, s := range statuses {
		state := s.State
		if state == "running" {
			state = pterm.Green(state)
		} else {
			state = pterm.Red(state)
		}

		name := s.Service
		if name == "" {
			name = s.Name
		}

		rows = append(rows, []string{name, state, s.Status, s.Ports})
	}

	ui.Table(headers, rows)

	return nil
}

// statFile is a helper that returns os.FileInfo or an error.
func statFile(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
