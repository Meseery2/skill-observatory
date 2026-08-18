package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/meseery/skill-observatory/internal/clierr"
	"github.com/meseery/skill-observatory/internal/eval/quality"
	"github.com/meseery/skill-observatory/internal/store"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newEvalQualityCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "quality [NAME]",
		Short: "Compare with-skill vs without-skill completions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			ctx := cmd.Context()
			st, err := openStore()
			if err != nil {
				return err
			}
			defer closeErr(st, &err)
			skills, err := loadInventory(ctx, st)
			if err != nil {
				return err
			}
			names := args
			if len(names) == 0 {
				names, err = listFixtureSkills("evals.json")
				if err != nil {
					return err
				}
				if len(names) == 0 {
					return clierr.Usage(fmt.Errorf("no evals.json fixtures under %s", evalsDir()))
				}
			}
			client, err := newLLM()
			if err != nil {
				return err
			}
			model := viper.GetString("model")
			if model == "" {
				model = viper.GetString("llm.model")
			}

			var results []quality.Result
			for _, name := range names {
				sk, err := findSkill(skills, name)
				if err != nil {
					return err
				}
				cases, err := quality.LoadCases(evalsDir(), name)
				if err != nil {
					return err
				}
				started := time.Now().UTC()
				res, err := quality.Run(ctx, quality.Options{
					Target: sk,
					Cases:  cases,
					Client: client,
				})
				if err != nil {
					return err
				}
				summary, err := json.Marshal(res.Summary)
				if err != nil {
					return err
				}
				var payloads []store.EvalResult
				for i, pair := range res.Pairs {
					raw, err := json.Marshal(pair)
					if err != nil {
						return err
					}
					payloads = append(payloads, store.EvalResult{
						CaseID:      pair.CaseID,
						Repetition:  i + 1,
						PayloadJSON: string(raw),
					})
				}
				if _, err := st.InsertEvalRun(ctx, store.EvalRun{
					Kind:        "quality",
					SkillName:   sk.Name,
					Model:       model,
					StartedAt:   started.Format(time.RFC3339),
					FinishedAt:  time.Now().UTC().Format(time.RFC3339),
					SummaryJSON: string(summary),
				}, payloads); err != nil {
					return err
				}
				results = append(results, res)
			}

			var table [][]string
			for _, r := range results {
				table = append(table, []string{
					r.Skill,
					strconv.Itoa(r.Summary.Cases),
					strconv.Itoa(r.Summary.WithWins),
					strconv.Itoa(r.Summary.WithoutWins),
					strconv.Itoa(r.Summary.Ties),
					fmt.Sprintf("%.2f", r.Summary.WinRate),
					strconv.Itoa(r.Summary.TokenDelta),
					strconv.FormatInt(r.Summary.LatencyDeltaMS, 10),
				})
			}
			return writeOutput(cmd, results, []string{"SKILL", "CASES", "WITH", "WITHOUT", "TIE", "WINRATE", "TOKΔ", "MSΔ"}, table)
		},
	}
}
