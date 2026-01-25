package main

import (
	"fmt"
	"os"
	"runtime/debug"

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

	rootCmd.AddCommand(versionCmd)

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
