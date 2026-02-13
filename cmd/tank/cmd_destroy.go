package main

import (
	"fmt"
	"os"

	"github.com/rhettg/tank/instance"
	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

func newDestroyCmd(projectPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "destroy [name]",
		Short: "Stop and remove the VM completely",
		Long:  "Destroy a VM instance (force stop and remove all files). Default name is the project directory name.",
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

			ui.PrintInfo(os.Stdout, "Destroying instance %s", ui.Bold.Render(instanceName))
			if err := inst.Destroy(os.Stdout); err != nil {
				return fmt.Errorf("destroying instance: %w", err)
			}

			ui.PrintSuccess(os.Stdout, "Instance %s destroyed", ui.Bold.Render(instanceName))
			return nil
		},
	}
}
