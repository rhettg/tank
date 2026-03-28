package main

import (
	"fmt"

	"github.com/rhettg/tank/build"
	"github.com/rhettg/tank/project"
	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

func newBuildCmd(projectPath *string) *cobra.Command {
	var dryRun bool
	var buildNoCache bool

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a VM image from project layers",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			p, err := project.Load(*projectPath)
			if err != nil {
				return fmt.Errorf("loading project: %w", err)
			}

			if dryRun {
				return build.PrintPlan(out, p)
			}

			if errs := build.Preflight(); build.PrintPreflightErrors(errs) {
				return fmt.Errorf("preflight checks failed")
			}

			buildImagePath, err := build.Build(p, out, build.BuildOptions{
				NoCache: buildNoCache,
				Verbose: verbose,
			})
			if err != nil {
				return fmt.Errorf("build: %w", err)
			}
			ui.PrintSuccess(out, "Build image ready: %s", ui.MutedStyle.Render(buildImagePath))

			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show build plan without executing")
	cmd.Flags().BoolVar(&buildNoCache, "no-cache", false, "rebuild image without using cached stages")

	return cmd
}
