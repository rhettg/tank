package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/rhettg/graystone/project"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "gi",
		Short: "Graystone Industries - deterministic VM images",
		Long:  "Build and run disposable virtual machines using libvirt and KVM.",
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gi version %s\n", getVersion())
		},
	}

	var projectPath string
	layersCmd := &cobra.Command{
		Use:   "layers",
		Short: "List project layers and their content hashes",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project.Load(projectPath)
			if err != nil {
				return fmt.Errorf("loading project: %w", err)
			}

			fmt.Printf("Base: %s\n", p.Base)
			fmt.Println("Layers:")
			for _, layer := range p.Layers {
				script := "       "
				if layer.HasScript {
					script = "[script]"
				}
				files := "       "
				if layer.HasFiles {
					files = "[files]"
				}
				fmt.Printf("  %-14s %s %s  %s\n", layer.Name, script, files, layer.ContentHash[:8])
			}
			return nil
		},
	}
	layersCmd.Flags().StringVarP(&projectPath, "project", "p", ".", "path to project directory")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(layersCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func getVersion() string {
	version := "dev"
	revision := ""
	dirty := false

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				dirty = setting.Value == "true"
			}
		}
	}

	if revision != "" {
		version = revision[:min(7, len(revision))]
		if dirty {
			version += "-dirty"
		}
	}

	return version
}
