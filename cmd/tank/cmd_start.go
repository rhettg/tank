package main

import (
	"github.com/spf13/cobra"
)

func newStartCmd(projectPath *string) *cobra.Command {
	var startCPUs int
	var startMemory int
	var startNoCache bool

	cmd := &cobra.Command{
		Use:   "start [name]",
		Short: "Start the VM (builds image if needed)",
		Long:  "Start a VM instance. Default name is the project directory name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceName := ""
			if len(args) > 0 {
				instanceName = args[0]
			}
			return ensureRunning(*projectPath, instanceName, startCPUs, startMemory, startNoCache)
		},
	}
	cmd.Flags().IntVar(&startCPUs, "cpus", 2, "number of CPUs")
	cmd.Flags().IntVar(&startMemory, "memory", 8192, "memory in MB")
	cmd.Flags().BoolVar(&startNoCache, "no-cache", false, "rebuild image without using cached stages")

	return cmd
}
