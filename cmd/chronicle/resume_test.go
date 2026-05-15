package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// TestWriteResumePrint_includesProviderAndCwd is the
// rendering-layer happy path. The printable form has to
// surface the provider, the working directory, and the
// exact command, plus a paste-ready shell line. All four
// pieces are what makes --print actually useful.
func TestWriteResumePrint_includesProviderAndCwd(t *testing.T) {
	result := composition.ResumeResult{
		Provider: "claude",
		Plan: contracts.ResumePlan{
			Command:    []string{"claude", "--resume", "abc-123"},
			WorkingDir: "/Users/x/work/repo",
		},
	}

	var buf bytes.Buffer
	if err := writeResumePrint(&buf, result); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"Provider:    claude",
		"WorkingDir:  /Users/x/work/repo",
		"Command:     claude --resume abc-123",
		"Shell:       cd /Users/x/work/repo && claude --resume abc-123",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing line %q in:\n%s", want, out)
		}
	}
}

// TestWriteResumePrint_quotesShellMetacharacters confirms
// the paste-safe shell line. A directory with spaces in it
// would break a naive cd command, so the shell-form has to
// quote it.
func TestWriteResumePrint_quotesShellMetacharacters(t *testing.T) {
	result := composition.ResumeResult{
		Provider: "claude",
		Plan: contracts.ResumePlan{
			Command:    []string{"claude", "--resume", "abc"},
			WorkingDir: "/Users/x/My Projects/repo",
		},
	}

	var buf bytes.Buffer
	if err := writeResumePrint(&buf, result); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, "cd '/Users/x/My Projects/repo'") {
		t.Errorf("output missing single-quoted cd target in:\n%s", out)
	}
}

// TestShellQuote_quotesOnlyWhenNeeded covers the small
// helper directly. We want unquoted output for the common
// case so the printed shell line stays readable, and
// quoted output only when the input contains a character
// that would otherwise change the shell's interpretation.
func TestShellQuote_quotesOnlyWhenNeeded(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/Users/x/work/repo", "/Users/x/work/repo"},
		{"/Users/x/My Projects", "'/Users/x/My Projects'"},
		{"abc-123", "abc-123"},
		{"abc 123", "'abc 123'"},
		{"a'b", "'a'\\''b'"},
	}
	for _, tc := range tests {
		if got := shellQuote(tc.in); got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestShellJoin_quotesPerToken proves each argv element is
// quoted independently. A single bad token must not
// destabilise the whole line, and a clean token next to a
// dirty one should still come through unquoted.
func TestShellJoin_quotesPerToken(t *testing.T) {
	got := shellJoin([]string{"claude", "--resume", "abc 123"})
	want := "claude --resume 'abc 123'"
	if got != want {
		t.Errorf("shellJoin = %q, want %q", got, want)
	}
}

// TestCheckWorkingDir_rejectsEmpty confirms the empty-cwd
// guard. Empty working directories slip through provider
// implementations that forget to fill in the field, and
// the symptom (chronicle silently chdirs to the user's
// shell cwd) would be very confusing. The explicit check
// turns it into a clean error.
func TestCheckWorkingDir_rejectsEmpty(t *testing.T) {
	if err := checkWorkingDir(""); err == nil {
		t.Error("expected an error for an empty working directory")
	}
}

// TestCheckWorkingDir_rejectsMissingPath proves the
// stat-before-exec contract. A user who deleted the
// project but kept the session on disk would otherwise see
// a cryptic syscall failure during exec, instead of the
// "was the project moved or deleted?" message this guard
// produces.
func TestCheckWorkingDir_rejectsMissingPath(t *testing.T) {
	if err := checkWorkingDir("/this/path/does/not/exist/chronicle/test"); err == nil {
		t.Error("expected an error for a missing working directory")
	}
}

// TestCheckWorkingDir_acceptsTempDir is the positive case.
// The standard temp directory exists and is a directory,
// so the check passes.
func TestCheckWorkingDir_acceptsTempDir(t *testing.T) {
	if err := checkWorkingDir(t.TempDir()); err != nil {
		t.Errorf("expected nil for an existing temp dir, got %v", err)
	}
}

// TestResumeFailure_translatesNotFound proves the
// fs.ErrNotExist branch turns into a friendly message that
// points the user at chronicle list. The fail() helper
// returns an error that wraps the printed string, so we
// assert on the error message rather than capturing
// stderr.
func TestResumeFailure_translatesNotFound(t *testing.T) {
	wrapped := fmt.Errorf("resume: %w", fs.ErrNotExist)
	err := resumeFailure(wrapped)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("err = %q, want a 'session not found' message", err)
	}
	if !strings.Contains(err.Error(), "chronicle list") {
		t.Errorf("err = %q, want a 'chronicle list' hint", err)
	}
}

// TestResumeFailure_translatesUnsupportedProvider proves
// the ErrResumeUnsupported branch tells the user what to
// do next ("reopen inside the underlying tool's UI").
func TestResumeFailure_translatesUnsupportedProvider(t *testing.T) {
	wrapped := fmt.Errorf("resume copilot: %w", composition.ErrResumeUnsupported)
	err := resumeFailure(wrapped)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Reopen this session") {
		t.Errorf("err = %q, want a 'Reopen this session' hint", err)
	}
}

// TestResumeFailure_passesThroughGenericErrors makes sure
// an error that does not match either sentinel still
// surfaces with a useful prefix instead of being swallowed
// by an over-specific switch.
func TestResumeFailure_passesThroughGenericErrors(t *testing.T) {
	err := resumeFailure(fmt.Errorf("disk on fire"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "disk on fire") {
		t.Errorf("err = %q, want it to mention the original cause", err)
	}
}
