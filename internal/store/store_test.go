package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/meseery/skill-observatory/internal/skill"
	"github.com/stretchr/testify/require"
)

func TestReplaceAndListSkills(t *testing.T) {
	t.Parallel()

	s := openTestStore(t)
	ctx := context.Background()
	err := s.ReplaceSkills(ctx, []skill.Skill{
		{
			Name:             "canvas",
			Description:      "Use when creating a canvas.",
			Path:             "/tmp/canvas/SKILL.md",
			Source:           skill.SourceBuiltin,
			DescriptionChars: 28,
			BodyLines:        10,
			ContentHash:      "abc",
			Flags:            []string{"no-trigger-terms"},
		},
	})
	require.NoError(t, err)

	got, err := s.ListSkills(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "canvas", got[0].Name)
	require.Equal(t, []string{"no-trigger-terms"}, got[0].Flags)
}

func TestInvocationsFilter(t *testing.T) {
	t.Parallel()

	s := openTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.ReplaceInvocations(ctx, []Invocation{
		{
			ConversationID: "c1",
			Project:        "p1",
			TurnIndex:      0,
			Prompt:         "optimize my resume",
			SkillName:      "resume-ats-optimizer",
			SkillPath:      "/x/SKILL.md",
			Kind:           "manual",
			InvokedAt:      time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		{
			ConversationID: "c2",
			Project:        "p2",
			TurnIndex:      0,
			Prompt:         "write go errors",
			SkillName:      "golang-error-handling",
			SkillPath:      "/y/SKILL.md",
			Kind:           "auto",
			InvokedAt:      time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
	}))

	got, err := s.ListInvocations(ctx, InvocationFilter{Skill: "resume-ats-optimizer"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "c1", got[0].ConversationID)
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}
