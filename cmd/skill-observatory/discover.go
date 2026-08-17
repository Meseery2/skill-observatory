package main

import (
	"fmt"
	"strconv"

	"github.com/meseery/skill-observatory/internal/discover"
	"github.com/spf13/cobra"
)

func newDiscoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "discover",
		Short: "Scan skill directories and upsert the inventory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			res, err := scanSkills()
			if err != nil {
				return err
			}
			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()
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
				fmt.Fprintln(cmd.ErrOrStderr(), "name collisions:")
				for _, d := range res.Duplicates {
					fmt.Fprintf(cmd.ErrOrStderr(), "  %s (%d copies)\n", d.Name, len(d.Paths))
				}
			}
			return nil
		},
	}
}
