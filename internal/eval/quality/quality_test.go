package quality

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/meseery/skill-observatory/internal/llm"
	"github.com/meseery/skill-observatory/internal/skill"
	"github.com/stretchr/testify/require"
)

func TestSummarizeWinRate(t *testing.T) {
	t.Parallel()

	s := summarize([]Pair{
		{Winner: "with"},
		{Winner: "with"},
		{Winner: "without"},
		{Winner: "tie"},
	})
	require.Equal(t, 2, s.WithWins)
	require.Equal(t, 1, s.WithoutWins)
	require.Equal(t, 1, s.Ties)
	require.InDelta(t, 2.0/3.0, s.WinRate, 1e-9)
}

func TestRun_blindJudgeMapsWinner(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	require.NoError(t, os.WriteFile(path, []byte("---\nname: demo\ndescription: demo\n---\n# Demo\nUse numbered steps.\n"), 0o644))

	client := &llm.Scripted{Responses: []llm.Response{
		{Text: "WITH SKILL OUTPUT", InputTokens: 10, OutputTokens: 5},
		{Text: "WITHOUT SKILL OUTPUT", InputTokens: 4, OutputTokens: 5},
		{Text: `{"winner":"a","reason":"A follows the skill.","assertions":[{"text":"numbered steps","passed_a":true,"passed_b":false,"evidence":"A has 1. 2. 3."}]}`},
	}}

	got, err := Run(context.Background(), Options{
		Target: skill.Skill{Name: "demo", Path: path, Dir: dir},
		Cases: []Case{{
			ID:             "steps",
			Prompt:         "explain how to toast bread",
			ExpectedOutput: "numbered steps",
			Assertions:     []string{"numbered steps"},
		}},
		Client: client,
	})
	require.NoError(t, err)
	require.Len(t, got.Pairs, 1)
	require.Contains(t, []string{"with", "without"}, got.Pairs[0].Winner)
	require.NotEmpty(t, got.Pairs[0].Reason)
}

func TestLoadCases_objectAndMissing(t *testing.T) {
	t.Parallel()

	cases, err := LoadCases("testdata", "demo")
	require.NoError(t, err)
	require.Equal(t, "q1", cases[0].ID)
	require.Equal(t, []string{"numbered steps"}, cases[0].Assertions)

	_, err = LoadCases("testdata", "missing")
	require.Error(t, err)
}
