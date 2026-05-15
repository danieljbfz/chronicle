package claude

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/danieljbfz/chronicle/contracts"
)

// sessionWithCwd builds a minimal JSONL body that has the
// shape ResumeCommand expects: a couple of header records
// followed by a record carrying the cwd field. The body is
// what Claude writes for a real session, condensed to the
// few fields the cwd-extractor cares about.
func sessionWithCwd(cwd string) []byte {
	header := `{"type":"summary","leafUuid":"abc"}` + "\n"
	mode := `{"type":"permissionMode","sessionId":"x"}` + "\n"
	user := `{"type":"user","cwd":"` + cwd + `"}` + "\n"
	return []byte(header + mode + user)
}

// TestResumeCommand_readsCwdFromSessionRecord is the happy
// path. The encoded folder name is ambiguous on purpose
// (claude-history both as a single folder and as
// claude/history would encode the same way), and the test
// proves the implementation reaches into the JSONL to get
// the authoritative cwd instead of running the lossy
// decoder.
func TestResumeCommand_readsCwdFromSessionRecord(t *testing.T) {
	const realCwd = "/Users/djbf/Desktop/work/claude-history"
	fsys := fstest.MapFS{
		"projects/-Users-djbf-Desktop-work-claude-history/" + validUUID + ".jsonl": {
			Data: sessionWithCwd(realCwd),
		},
	}

	plan, err := New().ResumeCommand(fsys, contracts.SessionID(validUUID))
	if err != nil {
		t.Fatal(err)
	}

	wantCmd := []string{"claude", "--resume", validUUID}
	if !reflect.DeepEqual(plan.Command, wantCmd) {
		t.Errorf("command = %v, want %v", plan.Command, wantCmd)
	}
	if plan.WorkingDir != realCwd {
		t.Errorf("working dir = %q, want %q (the cwd recorded in the session)", plan.WorkingDir, realCwd)
	}
}

// TestResumeCommand_unknownSessionReturnsNotExist proves the
// not-found contract. Composition relies on errors.Is(err,
// fs.ErrNotExist) to tell the difference between "this
// provider does not own that session" and a real failure,
// so the wrap has to chain cleanly.
func TestResumeCommand_unknownSessionReturnsNotExist(t *testing.T) {
	fsys := fstest.MapFS{
		"projects/-Users-test-foo/" + validUUID + ".jsonl": {
			Data: sessionWithCwd("/Users/test/foo"),
		},
	}

	_, err := New().ResumeCommand(fsys, contracts.SessionID(otherUUID))
	if err == nil {
		t.Fatal("expected an error for an unknown session id")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want one wrapping fs.ErrNotExist", err)
	}
}

// TestResumeCommand_fallsBackToFolderDecodeWhenNoCwd
// verifies the last-resort fallback. A session with zero
// records that carry a cwd should still produce a usable
// plan, derived from the encoded folder name. The result is
// lossy for paths whose components contained hyphens, and
// the test picks a folder name that does round-trip cleanly
// so the fallback's output is unambiguous.
func TestResumeCommand_fallsBackToFolderDecodeWhenNoCwd(t *testing.T) {
	noCwdBody := []byte(`{"type":"summary"}` + "\n" + `{"type":"permissionMode"}` + "\n")
	fsys := fstest.MapFS{
		"projects/-tmp-fixture/" + validUUID + ".jsonl": {Data: noCwdBody},
	}

	plan, err := New().ResumeCommand(fsys, contracts.SessionID(validUUID))
	if err != nil {
		t.Fatal(err)
	}
	if plan.WorkingDir != "/tmp/fixture" {
		t.Errorf("working dir = %q, want /tmp/fixture (folder-decode fallback)", plan.WorkingDir)
	}
}

// TestProjectFolderFromSessionPath_extractsTheRightSegment
// pins the small string-handling helper. We test it
// directly because the function is reused as the contract
// between the path layout and the decoder, and a mistake
// here would silently make the resume target the wrong
// directory.
func TestProjectFolderFromSessionPath_extractsTheRightSegment(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"projects/-Users-x-work/" + validUUID + ".jsonl", "-Users-x-work"},
		{"projects/-tmp/" + validUUID + ".jsonl", "-tmp"},
	}
	for _, tc := range tests {
		if got := projectFolderFromSessionPath(tc.in); got != tc.want {
			t.Errorf("projectFolderFromSessionPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
