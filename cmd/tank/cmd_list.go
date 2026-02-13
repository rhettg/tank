package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rhettg/tank/build"
	"github.com/rhettg/tank/instance"
	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list", "ps"},
		Short:   "List all VM instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			cacheDir, err := build.CacheDir()
			if err != nil {
				return err
			}

			instancesDir := filepath.Join(cacheDir, "instances")
			entries, err := os.ReadDir(instancesDir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println(ui.MutedStyle.Render("No instances found"))
					return nil
				}
				return err
			}

			var rows []ui.InstanceRow
			for _, entry := range entries {
				name := entry.Name()
				entryPath := filepath.Join(instancesDir, name)

				// Use os.Stat to follow symlinks
				info, err := os.Stat(entryPath)
				if err != nil || !info.IsDir() {
					continue
				}

				inst, err := instance.Load(name)
				if err != nil {
					continue
				}

				status := ui.FormatStatus(inst.IsRunning())

				ip := "-"
				if inst.IsRunning() {
					if addr, err := inst.IPAddress(); err == nil && addr != "" {
						ip = addr
					}
				}

				rows = append(rows, ui.InstanceRow{
					Name:   name,
					Status: status,
					IP:     ip,
				})
			}

			if len(rows) == 0 {
				fmt.Println(ui.MutedStyle.Render("No instances found"))
				return nil
			}

			fmt.Println(ui.RenderInstanceTable(rows))
			return nil
		},
	}
}
