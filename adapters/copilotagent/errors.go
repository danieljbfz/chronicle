package copilotagent

import "fmt"

// Error wraps a low-level operation failure with the
// adapter name, the operation that failed, and the path
// involved. The shape mirrors the error types in the
// claude and copilotchat packages so the user sees
// consistent error messages regardless of which adapter
// produced them.
type Error struct {
	Op   string
	Path string
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("copilot-agent: %s: %s", e.Op, e.Path)
	}
	if e.Path == "" {
		return fmt.Sprintf("copilot-agent: %s: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("copilot-agent: %s: %s: %v", e.Op, e.Path, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

// newError is the package-internal constructor every
// adapter method uses to wrap raw errors. Keeping the
// construction in one place means any future logging or
// tracing wired into the wrap point sees every site.
func newError(op, path string, err error) *Error {
	return &Error{Op: op, Path: path, Err: err}
}
