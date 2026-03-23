package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rhettg/tank/build"
	"github.com/rhettg/tank/instance"
	"github.com/rhettg/tank/project"
	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

func formatStatusBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func newStatusCmd(projectPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show project status",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project.Load(*projectPath)
			if err != nil {
				return fmt.Errorf("loading project: %w", err)
			}

			instanceName, err := resolveInstanceName(*projectPath, args)
			if err != nil {
				return err
			}

			projectHash := build.FinalBuildHash(p)
			buildCached := build.BuildImageExists(projectHash)
			baseCached := build.BaseImageCached(p.Base)
			pruneResult, err := build.AnalyzePrune()
			if err != nil {
				return fmt.Errorf("analyzing prune state: %w", err)
			}

			instExists := instance.Exists(instanceName)
			var inst *instance.Instance
			if instExists {
				inst, err = instance.Load(instanceName)
				if err != nil {
					return fmt.Errorf("loading instance: %w", err)
				}
			}

			state := ui.MutedStyle.Render("not created")
			ip := "-"
			if instExists {
				running := inst.IsRunning()
				state = ui.FormatStatus(running)
				if running {
					if addr, err := inst.IPAddress(); err == nil && addr != "" {
						ip = addr
					}
				}
			}

			cacheStatus := ui.WarningStyle.Render("missing")
			if buildCached {
				cacheStatus = ui.SuccessStyle.Render("cached")
			}

			instanceImageStatus := ui.MutedStyle.Render("none")
			if instExists {
				backingPath, err := backingFilePath(inst.DiskPath)
				if err != nil {
					instanceImageStatus = ui.MutedStyle.Render("unknown")
				} else {
					backingBase := strings.TrimSuffix(filepath.Base(backingPath), ".qcow2")
					backingShort := backingBase
					if len(backingShort) > 8 {
						backingShort = backingShort[:8]
					}
					if backingBase == projectHash {
						instanceImageStatus = ui.SuccessStyle.Render("fresh")
					} else {
						instanceImageStatus = ui.WarningStyle.Render(fmt.Sprintf("stale (built %s)", backingShort))
					}
				}
			}

			baseStatus := ui.WarningStyle.Render("missing")
			if baseCached {
				baseStatus = ui.SuccessStyle.Render("cached")
			}

			fmt.Printf("%s %s\n", ui.Bold.Render("Project:"), ui.MutedStyle.Render(p.Root))
			fmt.Printf("%s %s %s\n", ui.Bold.Render("Base:"), ui.Highlight.Render(p.Base), baseStatus)
			fmt.Printf("%s %s\n", ui.Bold.Render("Instance:"), ui.Highlight.Render(instanceName))
			fmt.Printf("%s %s\n", ui.Bold.Render("State:"), state)
			fmt.Printf("%s %s\n", ui.Bold.Render("IP:"), ip)
			fmt.Printf("%s %s %s\n", ui.Bold.Render("Build:"), ui.FormatHash(projectHash), cacheStatus)
			fmt.Printf("%s %s\n", ui.Bold.Render("Instance image:"), instanceImageStatus)
			if len(pruneResult.Reclaimable) == 0 {
				fmt.Printf("%s %s\n", ui.Bold.Render("Cache GC:"), ui.SuccessStyle.Render("no reclaimable builds"))
			} else {
				fmt.Printf("%s %s\n",
					ui.Bold.Render("Cache GC:"),
					ui.WarningStyle.Render(fmt.Sprintf("%d reclaimable build(s), %s", len(pruneResult.Reclaimable), formatStatusBytes(pruneResult.ReclaimableBytes))),
				)
			}
			fmt.Println()

			fmt.Printf("%s\n", ui.Bold.Render("Layers:"))
			if len(p.Layers) == 0 {
				fmt.Println(ui.MutedStyle.Render("No layers"))
			} else {
				for _, layer := range p.Layers {
					flags := []string{}
					if layer.HasScript {
						flags = append(flags, "install")
					}
					if layer.HasFiles {
						flags = append(flags, "files")
					}
					if layer.HasFirstboot {
						flags = append(flags, "firstboot")
					}
					if layer.HasPreboot {
						flags = append(flags, "preboot")
					}
					if len(layer.Volumes) > 0 {
						flags = append(flags, "volumes")
					}
					flagStr := ""
					if len(flags) > 0 {
						flagStr = " (" + strings.Join(flags, ", ") + ")"
					}
					fmt.Printf("  %s %s%s\n", ui.SymbolDot, ui.Bold.Render(layer.Name), flagStr)
					fmt.Printf("    %s\n", ui.MutedStyle.Render(layer.ContentHash[:8]))
				}
			}
			fmt.Println()

			blocks, networks, rootSize, err := project.CollectVolumes(p.Layers)
			if err != nil {
				return fmt.Errorf("collecting volumes: %w", err)
			}

			fmt.Printf("%s\n", ui.Bold.Render("Volumes:"))
			if len(blocks) == 0 && len(networks) == 0 && rootSize == "" {
				fmt.Println(ui.MutedStyle.Render("No volumes"))
				return nil
			}
			if rootSize != "" {
				fmt.Printf("  %s root %s\n", ui.SymbolDot, rootSize)
			}
			for _, vol := range blocks {
				owner := ""
				if vol.Owner != "" {
					owner = fmt.Sprintf(" owner:%s", vol.Owner)
				}
				fmt.Printf("  %s %s block %s → %s (%s%s)\n",
					ui.SymbolDot,
					ui.Bold.Render(vol.Name),
					vol.Size,
					vol.Mount,
					vol.Format,
					owner,
				)
			}
			for _, vol := range networks {
				opts := ""
				if vol.Options != "" {
					opts = " " + vol.Options
				}
				fmt.Printf("  %s %s %s %s → %s%s\n",
					ui.SymbolDot,
					ui.Bold.Render(vol.Name),
					vol.Type,
					vol.Source,
					vol.Mount,
					opts,
				)
			}
			return nil
		},
	}
}
