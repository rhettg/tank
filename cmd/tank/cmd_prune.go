package main

import (
	"github.com/rhettg/tank/build"
	"github.com/spf13/cobra"
)

func newPruneCmd() *cobra.Command {
	var apply bool

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove unreachable cached build artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := build.Prune(cmd.OutOrStdout(), build.PruneOptions{
				Apply: apply,
			})
			return err
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "delete unreachable build artifacts (default is dry-run)")

	return cmd
}
