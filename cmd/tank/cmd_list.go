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
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls", "ps"},
		Short:   "List VM instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			cacheDir, err := build.CacheDir()
			if err != nil {
				return err
			}

			instancesDir := filepath.Join(cacheDir, "instances")
			entries, err := os.ReadDir(instancesDir)
			if err != nil {
				if os.IsNotExist(err) {
					if jsonOutput {
						return writeJSON(out, []any{})
					}
					fmt.Fprintln(out, ui.MutedStyle.Render("No instances found"))
					return nil
				}
				return err
			}

			type instanceResult struct {
				Name    string `json:"name"`
				Running bool   `json:"running"`
				Status  string `json:"status"`
				IP      string `json:"ip,omitempty"`
			}

			var rows []ui.InstanceRow
			var results []instanceResult
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

				state := inst.State()
				running := state == "running"
				status := ui.FormatStatus(state)

				ip := "-"
				jsonIP := ""
				if running {
					if addr, err := inst.IPAddress(); err == nil && addr != "" {
						ip = addr
						jsonIP = addr
					}
				}

				rows = append(rows, ui.InstanceRow{
					Name:   name,
					Status: status,
					IP:     ip,
				})
				results = append(results, instanceResult{
					Name:    name,
					Running: running,
					Status:  map[string]string{"running": "running", "paused": "paused"}[state],
					IP:      jsonIP,
				})
				if results[len(results)-1].Status == "" {
					results[len(results)-1].Status = "stopped"
				}
			}

			if jsonOutput {
				return writeJSON(out, results)
			}

			if len(rows) == 0 {
				fmt.Fprintln(out, ui.MutedStyle.Render("No instances found"))
				return nil
			}

			fmt.Fprintln(out, ui.RenderInstanceTable(rows))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable JSON")
	return cmd
}
