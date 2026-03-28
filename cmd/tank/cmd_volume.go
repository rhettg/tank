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
	makeVolumeListCmd := func(use string, aliases []string, volumeListAll *bool, volumeListInstance *string, jsonOutput *bool) *cobra.Command {
		cmd := &cobra.Command{
			Use:     use,
			Aliases: aliases,
			Short:   "List volumes",
			Long:    "List persistent volumes. By default shows volumes for the current project's instances. Use --instance to scope to a single instance, or --all to include all volumes.",
			Args:    cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				var filter string
				emptyMessage := "No volumes found"
				switch {
				case *volumeListAll && *volumeListInstance != "":
					return fmt.Errorf("--all and --instance cannot be used together")
				case *volumeListInstance != "":
					filter = *volumeListInstance
					emptyMessage = fmt.Sprintf("No volumes found for instance %s", *volumeListInstance)
				case !*volumeListAll:
					// Use current project's directory name as instance filter
					absPath, err := filepath.Abs(".")
					if err != nil {
						return err
					}
					filter = filepath.Base(absPath)
					emptyMessage = "No volumes found for this project"
				}

				volumes, err := instance.ListVolumes(filter)
				if err != nil {
					return fmt.Errorf("listing volumes: %w", err)
				}

				type volumeResult struct {
					Name         string `json:"name"`
					InstanceName string `json:"instance_name"`
					VolumeName   string `json:"volume_name"`
					Path         string `json:"path"`
					SizeBytes    int64  `json:"size_bytes"`
					SizeHuman    string `json:"size_human"`
					Orphaned     bool   `json:"orphaned"`
				}

				var rows []ui.VolumeRow
				var results []volumeResult
				for _, vol := range volumes {
					// Format size
					sizeStr := formatVolumeSize(vol.Size)
					// Check if instance exists
					orphaned := !instance.Exists(vol.InstanceName)
					instanceStatus := vol.InstanceName
					if orphaned {
						instanceStatus = vol.InstanceName + " " + ui.MutedStyle.Render("(orphaned)")
					}

					rows = append(rows, ui.VolumeRow{
						Name:     vol.Name,
						Size:     sizeStr,
						Instance: instanceStatus,
						Path:     vol.Path,
					})
					results = append(results, volumeResult{
						Name:         vol.Name,
						InstanceName: vol.InstanceName,
						VolumeName:   vol.VolumeName,
						Path:         vol.Path,
						SizeBytes:    vol.Size,
						SizeHuman:    sizeStr,
						Orphaned:     orphaned,
					})
				}

				if *jsonOutput {
					return writeJSON(cmd.OutOrStdout(), results)
				}

				if len(rows) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), ui.MutedStyle.Render(emptyMessage))
					return nil
				}

				fmt.Fprintln(cmd.OutOrStdout(), ui.RenderVolumeTable(rows))
				return nil
			},
		}
		cmd.Flags().BoolVar(jsonOutput, "json", false, "print machine-readable JSON")
		return cmd
	}

	// Volume management commands
	volumeCmd := &cobra.Command{
		Use:   "volume",
		Short: "Manage persistent volumes",
	}

	var volumeListAll bool
	var volumeListInstance string
	var volumeListJSON bool
	volumeListCmd := makeVolumeListCmd("list", []string{"ls"}, &volumeListAll, &volumeListInstance, &volumeListJSON)
	volumeListCmd.Flags().BoolVar(&volumeListAll, "all", false, "list all volumes including orphaned")
	volumeListCmd.Flags().StringVar(&volumeListInstance, "instance", "", "list volumes for a single instance")

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
