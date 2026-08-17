package main

import (
	"fmt"
	"strconv"

	"github.com/meseery/skill-observatory/internal/eval/generate"
	"github.com/spf13/cobra"
)

func newEvalGenerateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "generate NAME",
		Short: "Draft trigger and quality fixtures for a skill via the LLM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			skills, err := loadInventory(ctx, st)
			if err != nil {
				return err
			}
			sk, err := findSkill(skills, args[0])
			if err != nil {
				return err
			}
			client, err := newLLM()
			if err != nil {
				return err
			}
			res, err := generate.Draft(ctx, client, sk, evalsDir())
			if err != nil {
				return err
			}
			payload := map[string]any{
				"skill":         sk.Name,
				"triggers_path": res.TriggersPath,
				"evals_path":    res.EvalsPath,
				"trigger_count": len(res.Triggers),
				"quality_count": len(res.Evals),
			}
			rows := [][]string{
				{"skill", sk.Name},
				{"triggers", strconv.Itoa(len(res.Triggers))},
				{"quality cases", strconv.Itoa(len(res.Evals))},
				{"triggers file", res.TriggersPath},
				{"evals file", res.EvalsPath},
			}
			if err := writeOutput(cmd, payload, []string{"FIELD", "VALUE"}, rows); err != nil {
				return err
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "review and edit the fixtures before scoring")
			return nil
		},
	}
}
