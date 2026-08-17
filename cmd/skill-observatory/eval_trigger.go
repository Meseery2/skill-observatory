package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/meseery/skill-observatory/internal/clierr"
	"github.com/meseery/skill-observatory/internal/eval/trigger"
	"github.com/meseery/skill-observatory/internal/store"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newEvalTriggerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trigger [NAME]",
		Short: "Score whether a skill description fires on the right prompts",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			mode, _ := cmd.Flags().GetString("catalog-mode")
			repeats, _ := cmd.Flags().GetInt("repeats")

			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			skills, err := loadInventory(ctx, st)
			if err != nil {
				return err
			}
			names := args
			if len(names) == 0 {
				names, err = listFixtureSkills("triggers.json")
				if err != nil {
					return err
				}
				if len(names) == 0 {
					return clierr.Usage(fmt.Errorf("no triggers.json fixtures under %s", evalsDir()))
				}
			}

			client, err := newLLM()
			if err != nil {
				return err
			}
			fullCatalog := trigger.CatalogFrom(skills, true)
			model := viper.GetString("model")
			if model == "" {
				model = viper.GetString("llm.model")
			}

			var results []trigger.Result
			for _, name := range names {
				sk, err := findSkill(skills, name)
				if err != nil {
					return err
				}
				if !sk.AutoInvocable() {
					fmt.Fprintf(cmd.ErrOrStderr(), "skipping %s: disable-model-invocation\n", name)
					continue
				}
				cases, err := trigger.LoadCases(evalsDir(), name)
				if err != nil {
					return err
				}
				catalog := fullCatalog
				if mode == "alone" {
					catalog = []trigger.CatalogEntry{{Name: sk.Name, Description: sk.Description}}
				}
				started := time.Now().UTC()
				res, err := trigger.Run(ctx, trigger.Options{
					Target:      sk,
					Catalog:     catalog,
					Cases:       cases,
					Repeats:     repeats,
					CatalogMode: mode,
					Client:      client,
				})
				if err != nil {
					return err
				}
				summary, err := json.Marshal(res.Metrics)
				if err != nil {
					return err
				}
				var payloads []store.EvalResult
				for _, trial := range res.Trials {
					raw, err := json.Marshal(trial)
					if err != nil {
						return err
					}
					payloads = append(payloads, store.EvalResult{
						CaseID:      trial.CaseID,
						Repetition:  trial.Repetition,
						PayloadJSON: string(raw),
					})
				}
				if _, err := st.InsertEvalRun(ctx, store.EvalRun{
					Kind:        "trigger",
					SkillName:   sk.Name,
					CatalogMode: mode,
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
					r.CatalogMode,
					fmt.Sprintf("%.2f", r.Metrics.Precision),
					fmt.Sprintf("%.2f", r.Metrics.Recall),
					fmt.Sprintf("%.2f", r.Metrics.F1),
					strconv.Itoa(r.Metrics.Clashes),
					strconv.Itoa(r.Metrics.Trials),
				})
			}
			return writeOutput(cmd, results, []string{"SKILL", "MODE", "PREC", "RECALL", "F1", "CLASH", "TRIALS"}, table)
		},
	}
	cmd.Flags().String("catalog-mode", "full", "catalog size: alone or full")
	cmd.Flags().Int("repeats", 3, "repetitions per case")
	return cmd
}
