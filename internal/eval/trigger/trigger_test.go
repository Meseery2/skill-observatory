package trigger

import (
	"context"
	"testing"

	"github.com/meseery/skill-observatory/internal/llm"
	"github.com/meseery/skill-observatory/internal/skill"
	"github.com/stretchr/testify/require"
)

func TestScore_precisionRecallClashes(t *testing.T) {
	t.Parallel()

	trials := []Trial{
		{ShouldTrigger: true, Selected: []string{"golang-error-handling"}, Hit: true},
		{ShouldTrigger: true, Selected: []string{"golang-testing"}, Clash: true},
		{ShouldTrigger: false, Selected: []string{"golang-error-handling"}, Hit: true},
		{ShouldTrigger: false, Selected: nil},
	}
	m := Score("golang-error-handling", trials)
	require.Equal(t, 1, m.TP)
	require.Equal(t, 1, m.FP)
	require.Equal(t, 1, m.TN)
	require.Equal(t, 1, m.FN)
	require.Equal(t, 1, m.Clashes)
	require.InDelta(t, 0.5, m.Precision, 1e-9)
	require.InDelta(t, 0.5, m.Recall, 1e-9)
}

func TestRun_usesRouterJSON(t *testing.T) {
	t.Parallel()

	client := &llm.Scripted{Responses: []llm.Response{
		{Text: `{"skills": ["golang-error-handling"]}`},
	}}
	got, err := Run(context.Background(), Options{
		Target: skill.Skill{Name: "golang-error-handling", Description: "Use when handling errors in Go."},
		Catalog: []CatalogEntry{
			{Name: "golang-error-handling", Description: "Use when handling errors in Go."},
		},
		Cases: []Case{
			{ID: "pos-1", Query: "wrap this os.Open error", ShouldTrigger: true},
		},
		Repeats:     1,
		CatalogMode: "alone",
		Client:      client,
	})
	require.NoError(t, err)
	require.Equal(t, 1, got.Metrics.TP)
	require.Equal(t, 1.0, got.Metrics.Recall)
}

func TestRun_rejectsSlashOnly(t *testing.T) {
	t.Parallel()

	_, err := Run(context.Background(), Options{
		Target: skill.Skill{Name: "deploy", DisableModelInvocation: true},
		Client: &llm.Scripted{},
	})
	require.Error(t, err)
}
