package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func execCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	viper.Reset()
	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestVersionCmd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	out, err := execCLI(t, "version")
	require.NoError(t, err)
	require.Contains(t, out, "skill-observatory")
}

func TestDiscoverAndInventoryJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(home, ".cursor", "skills", "demo-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: demo-skill
description: Use when running the demo skill in tests.
---
# Demo
`), 0o644))
	db := filepath.Join(t.TempDir(), "obs.db")

	out, err := execCLI(t, "discover", "--db", db, "--format", "json")
	require.NoError(t, err)
	var discovered map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &discovered))
	require.InDelta(t, 1, discovered["count"], 1e-9)

	out, err = execCLI(t, "inventory", "--db", db, "--format", "json")
	require.NoError(t, err)
	require.Contains(t, out, "demo-skill")
}

func TestUnknownFormat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	db := filepath.Join(t.TempDir(), "obs.db")
	_, err := execCLI(t, "inventory", "--db", db, "--format", "xml")
	require.Error(t, err)
}
