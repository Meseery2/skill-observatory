package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "tilde only", in: "~", want: home},
		{name: "tilde slash", in: "~/skill-observatory", want: filepath.Join(home, "skill-observatory")},
		{name: "absolute", in: "/tmp/db", want: "/tmp/db"},
		{name: "relative", in: "evals", want: "evals"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ExpandHome(tt.in); got != tt.want {
				t.Fatalf("ExpandHome(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
