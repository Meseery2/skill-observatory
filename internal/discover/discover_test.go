package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meseery/skill-observatory/internal/skill"
	"github.com/stretchr/testify/require"
)

func TestScan_userAndProjectRoots(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	project := t.TempDir()

	writeSkill(t, filepath.Join(home, ".cursor", "skills", "alpha", "SKILL.md"), "alpha", "Use when doing alpha work.")
	writeSkill(t, filepath.Join(home, ".claude", "skills", "alpha", "SKILL.md"), "alpha", "Use when doing alpha work.")
	writeSkill(t, filepath.Join(home, ".cursor", "skills-cursor", "canvas", "SKILL.md"), "canvas", "Use when creating a canvas.")
	writeSkill(t, filepath.Join(project, ".cursor", "skills", "deploy", "SKILL.md"), "deploy", "Use when deploying the app.")

	got, err := Scan(Options{Home: home, Projects: []string{project}})
	require.NoError(t, err)

	byName := map[string][]skill.Skill{}
	for _, s := range got.Skills {
		byName[s.Name] = append(byName[s.Name], s)
	}
	require.Len(t, byName["alpha"], 2)
	require.Equal(t, skill.SourceUser, byName["alpha"][0].Source)
	require.Equal(t, skill.SourceBuiltin, byName["canvas"][0].Source)
	require.Equal(t, skill.SourceProject, byName["deploy"][0].Source)
	require.Empty(t, got.Duplicates)
}

func TestScan_contentCollision(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".cursor", "skills", "alpha", "SKILL.md"), "alpha", "Use when doing alpha.")
	writeSkill(t, filepath.Join(home, ".agents", "skills", "alpha", "SKILL.md"), "alpha", "A completely different description for alpha.")

	got, err := Scan(Options{Home: home})
	require.NoError(t, err)
	require.Len(t, got.Duplicates, 1)
	require.Equal(t, "alpha", got.Duplicates[0].Name)
}

func TestDedupeByHash_prefersUser(t *testing.T) {
	t.Parallel()

	skills := []skill.Skill{
		{Name: "a", ContentHash: "h1", Source: skill.SourcePlugin, Path: "/plugin/a"},
		{Name: "a", ContentHash: "h1", Source: skill.SourceUser, Path: "/user/a"},
	}
	got := DedupeByHash(skills)
	require.Len(t, got, 1)
	require.Equal(t, skill.SourceUser, got[0].Source)
}

func writeSkill(t *testing.T, path, name, desc string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	body := "---\nname: " + name + "\ndescription: " + desc + "\n---\n# " + name + "\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
