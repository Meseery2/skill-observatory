package main

import (
	"strconv"

	"github.com/meseery/skill-observatory/internal/discover"
	"github.com/spf13/cobra"
)

func newDiscoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "discover",
		Short: "Scan skill directories and upsert the inventory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			ctx := cmd.Context()
			res, err := scanSkills()
			if err != nil {
				return err
			}
			st, err := openStore()
			if err != nil {
				return err
			}
			defer closeErr(st, &err)
			if err := st.ReplaceSkills(ctx, res.Skills); err != nil {
				return err
			}

			unique := discover.DedupeByHash(res.Skills)
			payload := map[string]any{
				"count":      len(res.Skills),
				"unique":     len(unique),
				"duplicates": res.Duplicates,
				"skills":     res.Skills,
			}
			rows := [][]string{
				{"files", strconv.Itoa(len(res.Skills))},
				{"unique", strconv.Itoa(len(unique))},
				{"name collisions", strconv.Itoa(len(res.Duplicates))},
			}
			if err := writeOutput(cmd, payload, []string{"METRIC", "VALUE"}, rows); err != nil {
				return err
			}
			if len(res.Duplicates) > 0 && outputFormat() == "table" {
				if err := writeErrln(cmd, "name collisions:"); err != nil {
					return err
				}
				for _, d := range res.Duplicates {
					if err := writeErrf(cmd, "  %s (%d copies)\n", d.Name, len(d.Paths)); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}
