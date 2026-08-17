package main

import (
	"fmt"
	"strconv"

	"github.com/meseery/skill-observatory/internal/fsutil"
	"github.com/meseery/skill-observatory/internal/report"
	"github.com/meseery/skill-observatory/internal/store"
	"github.com/meseery/skill-observatory/internal/transcript"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Join inventory, invocation log, and latest evals",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			outDir, _ := cmd.Flags().GetString("out")
			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			skills, err := loadInventory(ctx, st)
			if err != nil {
				return err
			}
			inv, err := st.ListInvocations(ctx, store.InvocationFilter{})
			if err != nil {
				return err
			}
			if len(inv) == 0 {
				root := fsutil.ExpandHome(viper.GetString("transcripts_root"))
				inv, err = transcript.ScanRoot(root)
				if err != nil {
					return err
				}
				if err := st.ReplaceInvocations(ctx, inv); err != nil {
					return err
				}
			}
			runs, err := st.LatestEvalRuns(ctx)
			if err != nil {
				return err
			}
			rep := report.Build(skills, inv, runs)
			htmlPath, err := report.WriteHTML(rep, outDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s\n", htmlPath)

			var table [][]string
			for _, row := range rep.Skills {
				f1 := "—"
				if row.TriggerF1 != nil {
					f1 = fmt.Sprintf("%.2f", *row.TriggerF1)
				}
				wr := "—"
				if row.QualityWinRate != nil {
					wr = fmt.Sprintf("%.2f", *row.QualityWinRate)
				}
				table = append(table, []string{
					row.Name,
					row.Source,
					strconv.Itoa(row.Invocations),
					f1,
					wr,
				})
			}
			return writeOutput(cmd, rep, []string{"SKILL", "SOURCE", "INVOKED", "TRIG F1", "QUAL WIN"}, table)
		},
	}
	cmd.Flags().String("out", "reports", "directory for the HTML report")
	return cmd
}
