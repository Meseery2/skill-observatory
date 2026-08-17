package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFile_manualAndAuto(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "empty-window", "agent-transcripts", "abc-123", "abc-123.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	jsonl := strings.Join([]string{
		`{"role":"user","message":{"content":[{"type":"text","text":"<manually_attached_skills>\nSkill Name: resume-ats-optimizer\nPath: /Users/x/.claude/skills/resume-ats-optimizer/SKILL.md\n</manually_attached_skills>\n<user_query>\noptimize my resume for ATS\n</user_query>"}]}}`,
		`{"role":"assistant","message":{"content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"Read","input":{"path":"/Users/x/.claude/skills/cover-letter-generator/SKILL.md"}}]}}`,
		`{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"path":"/Users/x/.claude/skills/cover-letter-generator/references/tone.md"}}]}}`,
	}, "\n")
	require.NoError(t, os.WriteFile(path, []byte(jsonl), 0o644))

	got, err := ParseFile(path)
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, "empty-window", got[0].Project)
	require.Equal(t, "abc-123", got[0].ConversationID)
	require.Equal(t, "optimize my resume for ATS", got[0].Prompt)
	require.Equal(t, "manual", got[0].Kind)
	require.Equal(t, "resume-ats-optimizer", got[0].SkillName)
	require.Equal(t, "auto", got[1].Kind)
	require.Equal(t, "cover-letter-generator", got[1].SkillName)
	require.Equal(t, "followon", got[2].Kind)
}

func TestParseFile_ignoresNonSkillAssets(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "proj", "agent-transcripts", "zzz", "zzz.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	jsonl := `{"role":"user","message":{"content":[{"type":"text","text":"<user_query>hi</user_query>"}]}}
{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"path":"/Users/x/app/assets/logo.png"}}]}}`
	require.NoError(t, os.WriteFile(path, []byte(jsonl), 0o644))
	got, err := ParseFile(path)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestScanRoot_skipsNonTranscripts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes.jsonl"), []byte("{}\n"), 0o644))
	got, err := ScanRoot(root)
	require.NoError(t, err)
	require.Empty(t, got)
}
