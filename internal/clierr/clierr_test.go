package clierr

import (
	"errors"
	"testing"
)

func TestUsageWrapsExitCode(t *testing.T) {
	t.Parallel()

	inner := errors.New("missing skill name")
	err := Usage(inner)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("code = %d, want 2", exitErr.Code)
	}
	if !errors.Is(err, inner) {
		t.Fatal("expected unwrap to inner error")
	}
	if exitErr.Error() != inner.Error() {
		t.Fatalf("Error() = %q", exitErr.Error())
	}
}

func TestExitErrorNilInner(t *testing.T) {
	t.Parallel()

	err := &ExitError{Code: 1}
	if err.Error() != "exit 1" {
		t.Fatalf("Error() = %q", err.Error())
	}
}
