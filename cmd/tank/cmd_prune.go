package main

import (
	"github.com/rhettg/tank/build"
	"github.com/spf13/cobra"
)

func newPruneCmd() *cobra.Command {
	var apply bool
	var explain string

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove unreachable cached build artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			if explain != "" {
				explanation, err := build.ExplainPrune(explain)
				if err != nil {
					return err
				}
				build.RenderPruneExplanation(cmd.OutOrStdout(), explanation)
				return nil
			}
			_, err := build.Prune(cmd.OutOrStdout(), build.PruneOptions{
				Apply: apply,
			})
			return err
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "delete unreachable build artifacts (default is dry-run)")
	cmd.Flags().StringVar(&explain, "explain", "", "explain why a cached build hash is kept or reclaimable")

	return cmd
}
