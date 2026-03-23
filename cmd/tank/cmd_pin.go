package main

import (
	"fmt"

	"github.com/rhettg/tank/build"
	"github.com/spf13/cobra"
)

func newPinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pin <build-hash>",
		Short: "Pin a cached build so prune keeps it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hash := args[0]
			if !build.BuildImageExists(hash) {
				return fmt.Errorf("cached build %q not found", hash)
			}
			if err := build.PinBuild(hash); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Pinned build %s\n", hash)
			return nil
		},
	}
}
