package commands

import (
	"fmt"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewLabRestartCommand creates the lab restart command.
func NewLabRestartCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	return &cobra.Command{
		Use:               "restart <service>",
		Short:             "Restart a lab service",
		Long:              `Restart a specific lab service.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeServices(configPath),
		RunE: func(cmd *cobra.Command, args []string) error {
			labCfg, cfgPath, err := config.LoadLabConfig(configPath)
			if err != nil {
				return err
			}

			orch, err := orchestrator.NewOrchestrator(log, labCfg, cfgPath)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			service := args[0]

			// Validate service name and provide helpful error
			if !orch.IsValidService(service) {
				ui.Error(fmt.Sprintf("Unknown service: %s", service))
				fmt.Println("\nAvailable services:")

				for _, s := range orch.GetValidServices() {
					fmt.Printf("  - %s\n", s)
				}

				return fmt.Errorf("unknown service: %s", service)
			}

			spinner := ui.NewSpinner(fmt.Sprintf("Restarting %s", service))

			if err := orch.Restart(cmd.Context(), service); err != nil {
				spinner.Fail(fmt.Sprintf("Failed to restart %s", service))

				return fmt.Errorf("failed to restart service: %w", err)
			}

			spinner.Success(fmt.Sprintf("%s restarted successfully", service))

			return nil
		},
	}
}
