package report

import (
	"os"
	"testing"

	"github.com/meseery/skill-observatory/internal/skill"
	"github.com/meseery/skill-observatory/internal/store"
	"github.com/stretchr/testify/require"
)

func TestBuild_deadAndHot(t *testing.T) {
	t.Parallel()

	f1 := 0.8
	skills := []skill.Skill{
		{Name: "canvas", Source: skill.SourceBuiltin, Description: "x"},
		{Name: "unused", Source: skill.SourceUser, Description: "y"},
	}
	inv := []store.Invocation{
		{SkillName: "canvas", Kind: "auto", Prompt: "make a comparison table", InvokedAt: "2026-08-01T00:00:00Z"},
		{SkillName: "canvas", Kind: "followon", Prompt: "make a comparison table", InvokedAt: "2026-08-01T00:00:00Z"},
	}
	runs := []store.EvalRun{
		{Kind: "trigger", SkillName: "canvas", CatalogMode: "full", SummaryJSON: `{"f1":0.8}`},
	}
	got := Build(skills, inv, runs)
	require.Len(t, got.Hot, 1)
	require.Equal(t, "canvas", got.Hot[0].Name)
	require.Equal(t, 1, got.Hot[0].Invocations)
	require.NotNil(t, got.Hot[0].TriggerF1)
	require.InDelta(t, f1, *got.Hot[0].TriggerF1, 1e-9)
	require.Len(t, got.Dead, 1)
	require.Equal(t, "unused", got.Dead[0].Name)
}

func TestWriteHTML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path, err := WriteHTML(Build(nil, nil, nil), dir)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "Skill Observatory")
}
