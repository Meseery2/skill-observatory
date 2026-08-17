package skill

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse_foldedDescriptionAndPaths(t *testing.T) {
	t.Parallel()

	raw := []byte(`---
name: canvas
description: >-
  A Cursor Canvas is a live React app.
  Use when producing a standalone analytical artifact.
paths:
  - "**/*.canvas.tsx"
disable-model-invocation: false
---
# Canvas

Body line one
Body line two
`)
	got, err := Parse("/tmp/canvas/SKILL.md", raw)
	require.NoError(t, err)
	require.Equal(t, "canvas", got.Name)
	require.Contains(t, got.Description, "standalone analytical artifact")
	require.Equal(t, []string{"**/*.canvas.tsx"}, got.Paths)
	require.False(t, got.DisableModelInvocation)
	require.Equal(t, 4, got.BodyLines)
	require.Len(t, got.ContentHash, 64)
}

func TestParse_commaSeparatedGlobsAndSlashOnly(t *testing.T) {
	t.Parallel()

	raw := []byte(`---
name: python-style
description: Style rules for Python files.
globs: "**/*.py, scripts/**/*.py"
disable-model-invocation: true
---
# Style
`)
	got, err := Parse("/x/python-style/SKILL.md", raw)
	require.NoError(t, err)
	require.Equal(t, []string{"**/*.py", "scripts/**/*.py"}, got.Paths)
	require.True(t, got.DisableModelInvocation)
	require.Contains(t, got.Flags, "slash-only")
}

func TestParse_nameFromDirectoryWhenMissing(t *testing.T) {
	t.Parallel()

	raw := []byte("# Just a body\n")
	got, err := Parse(filepath.Join("skills", "my-skill", "SKILL.md"), raw)
	require.NoError(t, err)
	require.Equal(t, "my-skill", got.Name)
	require.Contains(t, got.Flags, "empty-description")
}

func TestParse_unclosedFrontmatter(t *testing.T) {
	t.Parallel()

	_, err := Parse("SKILL.md", []byte("---\nname: x\n"))
	require.Error(t, err)
}
