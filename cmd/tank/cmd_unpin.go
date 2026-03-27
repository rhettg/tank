package main

import (
	"github.com/rhettg/tank/build"
	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

func newUnpinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpin <build-hash>",
		Short: "Remove a build pin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hash := args[0]
			if err := build.UnpinBuild(hash); err != nil {
				return err
			}
			ui.PrintSuccess(cmd.OutOrStdout(), "Unpinned build %s", ui.MutedStyle.Render(hash))
			return nil
		},
	}
}
