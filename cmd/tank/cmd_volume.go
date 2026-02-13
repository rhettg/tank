package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rhettg/tank/instance"
	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

func newVolumeCmd() *cobra.Command {
	makeVolumeListCmd := func(use string, aliases []string, volumeListAll *bool) *cobra.Command {
		return &cobra.Command{
			Use:     use,
			Aliases: aliases,
			Short:   "List volumes",
			Long:    "List persistent volumes. By default shows volumes for the current project's instances.",
			RunE: func(cmd *cobra.Command, args []string) error {
				var filter string
				if !*volumeListAll {
					// Use current project's directory name as instance filter
					absPath, err := filepath.Abs(".")
					if err != nil {
						return err
					}
					filter = filepath.Base(absPath)
				}

				volumes, err := instance.ListVolumes(filter)
				if err != nil {
					return fmt.Errorf("listing volumes: %w", err)
				}

				var rows []ui.VolumeRow
				for _, vol := range volumes {
					// Format size
					sizeStr := formatVolumeSize(vol.Size)
					// Check if instance exists
					instanceStatus := vol.InstanceName
					if !instance.Exists(vol.InstanceName) {
						instanceStatus = vol.InstanceName + " " + ui.MutedStyle.Render("(orphaned)")
					}

					rows = append(rows, ui.VolumeRow{
						Name:     vol.Name,
						Size:     sizeStr,
						Instance: instanceStatus,
						Path:     vol.Path,
					})
				}

				if len(rows) == 0 {
					if *volumeListAll {
						fmt.Println(ui.MutedStyle.Render("No volumes found"))
					} else {
						fmt.Println(ui.MutedStyle.Render("No volumes found for this project"))
					}
					return nil
				}

				fmt.Println(ui.RenderVolumeTable(rows))
				return nil
			},
		}
	}

	// Volume management commands
	volumeCmd := &cobra.Command{
		Use:   "volume",
		Short: "Manage persistent volumes",
	}

	var volumeListAll bool
	volumeListCmd := makeVolumeListCmd("ls", []string{"list"}, &volumeListAll)
	volumeListCmd.Flags().BoolVar(&volumeListAll, "all", false, "list all volumes including orphaned")

	var volumeRmForce bool
	volumeRmCmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a volume",
		Long:  "Remove a persistent volume by its full name (e.g. myproject-pgdata). Requires confirmation.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fullName := args[0]

			// Confirm deletion
			fmt.Printf("Remove volume %s? [y/N] ", ui.Bold.Render(fullName))
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				fmt.Println("Cancelled")
				return nil
			}

			if err := instance.RemoveVolume(fullName, volumeRmForce); err != nil {
				return fmt.Errorf("removing volume: %w", err)
			}

			ui.PrintSuccess(os.Stdout, "Volume %s removed", ui.Bold.Render(fullName))
			return nil
		},
	}
	volumeRmCmd.Flags().BoolVar(&volumeRmForce, "force", false, "force removal even if attached to a running instance")

	volumeCmd.AddCommand(volumeListCmd)
	volumeCmd.AddCommand(volumeRmCmd)

	return volumeCmd
}
