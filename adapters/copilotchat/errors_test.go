package copilotchat

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

// TestError_Error pins the documented "copilot: <op>: <path>:
// <err>" message format. The format is part of the user-facing
// contract (it shows up in CLI output and structured logs).
func TestError_Error(t *testing.T) {
	cases := []struct {
		name string
		e    *Error
		want string
	}{
		{"op only", &Error{Op: "detect"}, "copilot: detect"},
		{"op and err", &Error{Op: "detect", Err: errors.New("nope")}, "copilot: detect: nope"},
		{"op and path", &Error{Op: "read", Path: "workspaceStorage/abc/chatSessions/s.jsonl"}, "copilot: read: workspaceStorage/abc/chatSessions/s.jsonl"},
		{"all three", &Error{Op: "read", Path: "ws/s.jsonl", Err: errors.New("permission denied")}, "copilot: read: ws/s.jsonl: permission denied"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.e.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestError_UnwrapEnablesErrorsIs proves the Unwrap method makes
// errors.Is walk through the chain. Without Unwrap, callers
// could not test for the underlying sentinel error.
func TestError_UnwrapEnablesErrorsIs(t *testing.T) {
	wrapped := newError("read", "missing.jsonl", fs.ErrNotExist)
	if !errors.Is(wrapped, fs.ErrNotExist) {
		t.Error("errors.Is should walk through Error.Unwrap to find fs.ErrNotExist")
	}
}

// TestError_AsExtractsTypedError proves the typed error is
// reachable through errors.As, so the doctor view can read the
// Op field for context-aware messages.
func TestError_AsExtractsTypedError(t *testing.T) {
	wrapped := newError("detect", "broken.jsonl", errors.New("no JSON"))
	var typed *Error
	if !errors.As(wrapped, &typed) {
		t.Fatal("errors.As should extract the typed error")
	}
	if typed.Op != "detect" || typed.Path != "broken.jsonl" {
		t.Errorf("extracted Error has wrong fields: %+v", typed)
	}
	if !strings.Contains(typed.Error(), "no JSON") {
		t.Error("extracted Error should preserve the underlying message")
	}
}
