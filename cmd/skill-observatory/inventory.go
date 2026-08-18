package main

import (
	"strconv"

	"github.com/spf13/cobra"
)

func newInventoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inventory",
		Short: "List installed skills, source, and description flags",
		Args:  cobra.NoArgs,
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
			collisions := uniqueNames(skills)
			type row struct {
				Name       string   `json:"name"`
				Source     string   `json:"source"`
				SlashOnly  bool     `json:"slash_only"`
				Flags      []string `json:"flags"`
				Path       string   `json:"path"`
				BodyLines  int      `json:"body_lines"`
				DescChars  int      `json:"description_chars"`
				Collisions int      `json:"content_variants"`
			}
			out := make([]row, 0, len(skills))
			var table [][]string
			for _, s := range skills {
				flags := append([]string(nil), s.Flags...)
				if collisions[s.Name] > 1 {
					flags = append(flags, "name-collision")
				}
				out = append(out, row{
					Name:       s.Name,
					Source:     string(s.Source),
					SlashOnly:  s.DisableModelInvocation,
					Flags:      flags,
					Path:       s.Path,
					BodyLines:  s.BodyLines,
					DescChars:  s.DescriptionChars,
					Collisions: collisions[s.Name],
				})
				auto := "auto"
				if s.DisableModelInvocation {
					auto = "slash"
				}
				table = append(table, []string{
					s.Name,
					string(s.Source),
					auto,
					strconv.Itoa(s.BodyLines),
					joinFlags(flags),
					s.Path,
				})
			}
			return writeOutput(cmd, out, []string{"NAME", "SOURCE", "INVOKE", "LINES", "FLAGS", "PATH"}, table)
		},
	}
}
