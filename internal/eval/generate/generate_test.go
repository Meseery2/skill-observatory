package generate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/meseery/skill-observatory/internal/llm"
	"github.com/meseery/skill-observatory/internal/skill"
	"github.com/stretchr/testify/require"
)

func TestDraft_writesFixtures(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillPath := filepath.Join(dir, "demo", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
	require.NoError(t, os.WriteFile(skillPath, []byte("---\nname: demo\ndescription: Use when demoing.\n---\n# Demo\n"), 0o644))

	client := &llm.Scripted{Responses: []llm.Response{{
		Text: `{"triggers":[{"id":"pos-1","query":"please demo this","should_trigger":true}],"evals":[{"id":"q1","prompt":"demo","expected_output":"ok","assertions":["ok"]}]}`,
	}}}
	evalsDir := filepath.Join(dir, "evals")
	got, err := Draft(context.Background(), client, skill.Skill{
		Name:        "demo",
		Description: "Use when demoing.",
		Path:        skillPath,
	}, evalsDir)
	require.NoError(t, err)
	require.FileExists(t, got.TriggersPath)
	require.FileExists(t, got.EvalsPath)
	require.Len(t, got.Triggers, 1)
}
