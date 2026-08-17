package fsutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome expands a leading ~/ to the current user's home directory.
func ExpandHome(p string) string {
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return home
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
