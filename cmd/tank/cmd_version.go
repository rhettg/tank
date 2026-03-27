package main

import (
	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			ui.PrintKeyValue(cmd.OutOrStdout(), "tank", ui.Highlight.Render(getVersion()))
		},
	}
}
