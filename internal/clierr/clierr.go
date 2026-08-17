package clierr

import "fmt"

// ExitError maps a CLI failure to a Unix exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

func Usage(err error) error {
	return &ExitError{Code: 2, Err: err}
}
