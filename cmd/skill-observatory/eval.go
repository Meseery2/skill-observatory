package main

import (
	"github.com/spf13/cobra"
)

func newEvalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Run or generate skill evaluations",
	}
	cmd.AddCommand(newEvalGenerateCmd(), newEvalTriggerCmd(), newEvalQualityCmd())
	return cmd
}
