package main

import (
	"os"

	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "tank",
		Short: ui.Title.Render("Tank") + " — deterministic VM images",
		Long:  "Build and run virtual machines using libvirt and KVM.",
	}

	var projectPath string
	rootCmd.PersistentFlags().StringVarP(&projectPath, "project", "p", ".", "path to project directory")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newInitCmd(&projectPath))
	rootCmd.AddCommand(newLayersCmd(&projectPath))
	rootCmd.AddCommand(newBuildCmd(&projectPath))
	rootCmd.AddCommand(newStartCmd(&projectPath))
	rootCmd.AddCommand(newStopCmd(&projectPath))
	rootCmd.AddCommand(newDestroyCmd(&projectPath))
	rootCmd.AddCommand(newSSHCmd(&projectPath))
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newStatusCmd(&projectPath))
	rootCmd.AddCommand(newVolumeCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
