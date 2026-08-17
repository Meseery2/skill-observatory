package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/meseery/skill-observatory/internal/clierr"
	"github.com/meseery/skill-observatory/internal/fsutil"
	"github.com/meseery/skill-observatory/internal/store"
	"github.com/meseery/skill-observatory/internal/transcript"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newLogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show skill invocations mined from Cursor transcripts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			skillName, _ := cmd.Flags().GetString("skill")
			project, _ := cmd.Flags().GetString("project-filter")
			sinceRaw, _ := cmd.Flags().GetString("since")
			refresh, _ := cmd.Flags().GetBool("refresh")

			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			if refresh || mustRefreshInvocations(ctx, st) {
				root := fsutil.ExpandHome(viper.GetString("transcripts_root"))
				events, err := transcript.ScanRoot(root)
				if err != nil {
					return err
				}
				if err := st.ReplaceInvocations(ctx, events); err != nil {
					return err
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "indexed %d invocation events from %s\n", len(events), root)
			}

			var since time.Time
			if sinceRaw != "" {
				since, err = parseSince(sinceRaw)
				if err != nil {
					return clierr.Usage(err)
				}
			}
			events, err := st.ListInvocations(ctx, store.InvocationFilter{
				Skill:   skillName,
				Project: project,
				Since:   since,
			})
			if err != nil {
				return err
			}

			var table [][]string
			for _, e := range events {
				prompt := e.Prompt
				if len([]rune(prompt)) > 80 {
					prompt = string([]rune(prompt)[:80]) + "…"
				}
				prompt = strings.ReplaceAll(prompt, "\n", " ")
				conv := e.ConversationID
				if len(conv) > 8 {
					conv = conv[:8]
				}
				table = append(table, []string{
					e.Project,
					conv,
					strconv.Itoa(e.TurnIndex),
					e.Kind,
					e.SkillName,
					prompt,
				})
			}
			return writeOutput(cmd, events, []string{"PROJECT", "CONV", "TURN", "KIND", "SKILL", "PROMPT"}, table)
		},
	}
	cmd.Flags().String("skill", "", "filter by skill name")
	cmd.Flags().String("project-filter", "", "filter by Cursor project folder name")
	cmd.Flags().String("since", "", "only events at or after this time (RFC3339 or 7d, 24h)")
	cmd.Flags().Bool("refresh", true, "re-scan transcripts before listing")
	return cmd
}

func mustRefreshInvocations(ctx context.Context, st *store.Store) bool {
	events, err := st.ListInvocations(ctx, store.InvocationFilter{})
	return err != nil || len(events) == 0
}

func parseSince(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	if dur, err := time.ParseDuration(raw); err == nil {
		return time.Now().Add(-dur), nil
	}
	if strings.HasSuffix(raw, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid --since %q", raw)
		}
		return time.Now().Add(-time.Duration(n) * 24 * time.Hour), nil
	}
	return time.Time{}, fmt.Errorf("invalid --since %q (want RFC3339, duration, or Nd)", raw)
}
