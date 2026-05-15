package copilotchat

import "fmt"

// Error is the typed error every public function in this package
// returns when something goes wrong. We use a typed error instead
// of plain fmt.Errorf strings for two reasons.
//
// First, callers can use errors.As to extract the structured
// fields. The doctor view, for example, uses Error.Op to print
// "copilot: list" instead of just "list", which makes a multi-
// provider listing easier to read.
//
// Second, having one place to construct adapter errors means
// future improvements (adding a stack trace, tagging with the
// affected file path, integrating with a structured logger) are
// one-file changes instead of a scatter through every call site.
type Error struct {
	// Op is a short verb describing what was being done when the
	// error happened, like "detect" or "read session".
	Op string

	// Path is the relative path inside the Copilot root that
	// caused the trouble, when one applies. Stays empty for
	// errors that are not tied to a single file.
	Path string

	// Err is the underlying error. Wrapping it through this
	// field is what makes errors.Is and errors.As work all the
	// way through the chain.
	Err error
}

// Error implements the standard error interface. The format is
// "copilot: <op>: <path>: <err>" with the path piece omitted when
// none was set, so the message reads naturally either way.
func (e *Error) Error() string {
	switch {
	case e.Path == "" && e.Err == nil:
		return "copilot: " + e.Op
	case e.Path == "":
		return fmt.Sprintf("copilot: %s: %v", e.Op, e.Err)
	case e.Err == nil:
		return fmt.Sprintf("copilot: %s: %s", e.Op, e.Path)
	default:
		return fmt.Sprintf("copilot: %s: %s: %v", e.Op, e.Path, e.Err)
	}
}

// Unwrap returns the underlying error so the standard library
// errors.Is and errors.As helpers can walk through the chain.
// Without this method, callers could not test for an underlying
// fs.ErrNotExist or any other sentinel.
func (e *Error) Unwrap() error { return e.Err }

// newError is the small factory adapter functions use to build
// an Error. Putting it in one place keeps the format consistent
// and gives us a single seam to extend later (timestamps, hostnames,
// whatever future versions need).
func newError(op, path string, err error) *Error {
	return &Error{Op: op, Path: path, Err: err}
}
