package main

import (
	"fmt"

	"github.com/rhettg/tank/build"
	"github.com/rhettg/tank/project"
	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

func newLayersCmd(projectPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "layers",
		Short: "List project layers and their content hashes",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project.Load(*projectPath)
			if err != nil {
				return fmt.Errorf("loading project: %w", err)
			}

			_, _, rootSize, err := project.CollectVolumes(p.Layers)
			if err != nil {
				return fmt.Errorf("collecting volumes: %w", err)
			}
			resolvedRootSize, err := build.ResolveRootSize(rootSize)
			if err != nil {
				return err
			}

			stages := p.BuildChain(resolvedRootSize)
			cacheByLayer := make(map[string]ui.LayerCacheStatus)
			if len(stages) > 0 {
				for _, stage := range stages[1:] {
					if stage.Layer == nil {
						continue
					}
					if build.BuildImageExists(stage.Hash) {
						cacheByLayer[stage.Layer.Name] = ui.LayerCacheCached
					} else if stage.Layer.HasScript || stage.Layer.HasFiles || stage.Layer.HasFirstboot {
						cacheByLayer[stage.Layer.Name] = ui.LayerCacheBuild
					}
				}
			}

			baseCached := build.BaseImageCached(p.Base)

			rows := make([]ui.LayerRow, len(p.Layers))
			for i, layer := range p.Layers {
				cacheStatus := cacheByLayer[layer.Name]
				if cacheStatus == "" {
					if layer.HasScript || layer.HasFiles || layer.HasFirstboot {
						cacheStatus = ui.LayerCacheBuild
					} else {
						cacheStatus = ui.LayerCacheNA
					}
				}

				rows[i] = ui.LayerRow{
					Name:    layer.Name,
					Script:  layer.HasScript,
					Files:   layer.HasFiles,
					Volumes: len(layer.Volumes) > 0,
					Hash:    layer.ContentHash,
					Cache:   cacheStatus,
				}
			}
			fmt.Println(ui.RenderLayerTable(p.Base, baseCached, rows))
			return nil
		},
	}
}
