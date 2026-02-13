package main

import (
	"fmt"
	"os"

	"github.com/rhettg/tank/instance"
	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

func newStopCmd(projectPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "stop [name]",
		Short: "Stop the VM",
		Long:  "Stop a VM instance (graceful shutdown). Default name is the project directory name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine instance name
			instanceName, err := resolveInstanceName(*projectPath, args)
			if err != nil {
				return err
			}

			inst, err := instance.Load(instanceName)
			if err != nil {
				return fmt.Errorf("loading instance: %w", err)
			}

			if !inst.IsRunning() {
				ui.PrintInfo(os.Stdout, "Instance %s is not running", ui.Bold.Render(instanceName))
				return nil
			}

			if err := inst.Stop(os.Stdout); err != nil {
				return fmt.Errorf("stopping VM: %w", err)
			}

			ui.PrintSuccess(os.Stdout, "Instance %s stopping", ui.Bold.Render(instanceName))
			return nil
		},
	}
}
