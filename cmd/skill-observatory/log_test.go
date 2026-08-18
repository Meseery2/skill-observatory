package main

import (
	"testing"
	"time"
)

func TestParseSince(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantErr bool
		approx  time.Duration
	}{
		{name: "rfc3339", in: "2026-08-01T00:00:00Z"},
		{name: "hour duration", in: "24h", approx: 24 * time.Hour},
		{name: "days", in: "7d", approx: 7 * 24 * time.Hour},
		{name: "invalid", in: "yesterday", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseSince(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.approx != 0 {
				delta := time.Since(got)
				if delta < tt.approx-time.Minute || delta > tt.approx+time.Minute {
					t.Fatalf("got offset %s, want ~%s", delta, tt.approx)
				}
			}
		})
	}
}
