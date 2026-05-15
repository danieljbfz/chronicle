package claude

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// fixtureClaudeJSON is a representative .claude.json body
// the global-config tests share. The shape mirrors what
// a real Claude install writes: a userID at top level (we
// must never touch this), a projects map keyed by absolute
// directory path (some entries refer to directories that
// exist, others do not), and a few sibling keys (we must
// never touch these either). The indentation is the
// 2-space style Claude itself uses, which the tests below
// verify is preserved after edits.
const fixtureClaudeJSON = `{
  "userID": "ab33eb830e1a794768d77f13a6fe1d7e819964761b6f365082c62cf90ffe0534",
  "projects": {
    "%KEEP_PATH%": {
      "hasTrustDialogAccepted": true,
      "lastSessionId": "kept-session-id",
      "lastSessionMetrics": {"totalTokens": 12345}
    },
    "%GONE_PATH%": {
      "hasTrustDialogAccepted": false,
      "lastSessionId": "gone-session-id"
    }
  },
  "lastReleaseNotesSeen": "2.1.142",
  "numStartups": 93
}`

// writeFixture builds a temp home dir with a .claude.json
// file inside, where the placeholders have been replaced
// with real paths the test controls. The "keep" path
// resolves to a real directory; the "gone" path is a
// constructed absolute path that does not exist.
func writeFixture(t *testing.T) (homeDir, keepPath, gonePath string) {
	t.Helper()
	homeDir = t.TempDir()
	keepPath = filepath.Join(homeDir, "real-project")
	if err := os.MkdirAll(keepPath, 0o755); err != nil {
		t.Fatal(err)
	}
	gonePath = filepath.Join(homeDir, "this-project-was-deleted")

	body := strings.ReplaceAll(fixtureClaudeJSON, "%KEEP_PATH%", keepPath)
	body = strings.ReplaceAll(body, "%GONE_PATH%", gonePath)

	if err := os.WriteFile(filepath.Join(homeDir, globalConfigFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return
}

// TestListConfigProjectEntries_marksGonePathsAsStale is
// the happy path. Two entries: one whose key is a real
// directory, one whose key is missing. The Exists flag
// must distinguish them. The size measurements should
// reflect the actual on-disk byte length, not a re-encoded
// estimate.
func TestListConfigProjectEntries_marksGonePathsAsStale(t *testing.T) {
	homeDir, keepPath, gonePath := writeFixture(t)
	p := NewWithHome(homeDir)

	entries, err := p.ListConfigProjectEntries(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}

	byKey := map[string]bool{}
	for _, e := range entries {
		byKey[e.Key] = e.Exists
		if e.SizeBytes <= 0 {
			t.Errorf("size for %q = %d, want positive", e.Key, e.SizeBytes)
		}
	}
	if got, want := byKey[keepPath], true; got != want {
		t.Errorf("Exists for keep path = %v, want %v", got, want)
	}
	if got, want := byKey[gonePath], false; got != want {
		t.Errorf("Exists for gone path = %v, want %v", got, want)
	}
}

// TestListConfigProjectEntries_missingFileReturnsEmpty
// covers the fresh-install path. A user who has never
// launched Claude has no .claude.json and should get
// (nil, nil) instead of an error.
func TestListConfigProjectEntries_missingFileReturnsEmpty(t *testing.T) {
	p := NewWithHome(t.TempDir())
	entries, err := p.ListConfigProjectEntries(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0", len(entries))
	}
}

// TestListConfigProjectEntries_malformedReturnsError
// confirms we fail loudly on an unparseable file. Silently
// returning empty would let a corrupted file masquerade as
// a clean install.
func TestListConfigProjectEntries_malformedReturnsError(t *testing.T) {
	homeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(homeDir, globalConfigFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewWithHome(homeDir)
	if _, err := p.ListConfigProjectEntries(fstest.MapFS{}); err == nil {
		t.Error("expected an error for malformed JSON")
	}
}

// TestListConfigProjectEntries_failsWithoutHomeDir pins
// the New-vs-NewWithHome guard. A Provider built with the
// short constructor cannot reach a global config file, so
// the method must fail explicitly rather than silently
// reading from some default path.
func TestListConfigProjectEntries_failsWithoutHomeDir(t *testing.T) {
	p := New()
	_, err := p.ListConfigProjectEntries(fstest.MapFS{})
	if !errors.Is(err, errMissingHomeDir) {
		t.Errorf("err = %v, want errMissingHomeDir", err)
	}
}

// TestRemoveConfigProjectEntries_preservesUntouchedFields
// is the load-bearing test for the byte-preservation
// property. We delete one entry from the projects map and
// confirm that:
//   - the deleted entry is gone
//   - the kept entry is byte-identical
//   - the surrounding top-level keys (userID, sibling
//     fields) appear in the same byte sequence as before
//   - the indentation style (2 spaces, Claude's own
//     convention) is preserved
//
// This is the test that justifies the sjson dependency.
// A naive encoding/json round-trip would reformat the
// whole file even though only one key changed.
func TestRemoveConfigProjectEntries_preservesUntouchedFields(t *testing.T) {
	homeDir, keepPath, gonePath := writeFixture(t)
	p := NewWithHome(homeDir)

	backup, err := p.RemoveConfigProjectEntries(fstest.MapFS{}, []string{gonePath})
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Error("expected a non-empty backup path")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Errorf("backup file missing: %v", err)
	}

	edited, err := os.ReadFile(filepath.Join(homeDir, globalConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	got := string(edited)

	// The deleted entry must be gone.
	if strings.Contains(got, "this-project-was-deleted") {
		t.Errorf("deleted key still present in output:\n%s", got)
	}
	if strings.Contains(got, "gone-session-id") {
		t.Errorf("deleted entry value still present in output:\n%s", got)
	}

	// Kept entry must survive verbatim. We check for the
	// exact substring with the keep path interpolated, which
	// proves both the key and the nested object are intact.
	keepLine := `"` + keepPath + `"`
	if !strings.Contains(got, keepLine) {
		t.Errorf("kept entry missing %q in output:\n%s", keepLine, got)
	}
	if !strings.Contains(got, `"lastSessionId": "kept-session-id"`) {
		t.Errorf("kept entry's nested fields lost their formatting:\n%s", got)
	}

	// Top-level fields outside the projects map must
	// survive verbatim.
	for _, untouched := range []string{
		`"userID": "ab33eb830e1a794768d77f13a6fe1d7e819964761b6f365082c62cf90ffe0534"`,
		`"lastReleaseNotesSeen": "2.1.142"`,
		`"numStartups": 93`,
	} {
		if !strings.Contains(got, untouched) {
			t.Errorf("top-level field %q lost or reformatted:\n%s", untouched, got)
		}
	}
}

// TestRemoveConfigProjectEntries_handlesMissingKeysSilently
// confirms the contract: a key that is not in the file
// produces no error. This matters because the dry-run plan
// and the apply step happen at different times, and a key
// that was stale at plan time but got removed by another
// process by apply time should not block the rest of the
// removals.
func TestRemoveConfigProjectEntries_handlesMissingKeysSilently(t *testing.T) {
	homeDir, _, gonePath := writeFixture(t)
	p := NewWithHome(homeDir)

	if _, err := p.RemoveConfigProjectEntries(fstest.MapFS{}, []string{gonePath, "/never/existed"}); err != nil {
		t.Errorf("removing a never-existed key should not error, got %v", err)
	}
}

// TestRemoveConfigProjectEntries_writesBackupBeforeEditing
// confirms the safety order. Even when the edit succeeds,
// the backup must exist and contain the original bytes.
func TestRemoveConfigProjectEntries_writesBackupBeforeEditing(t *testing.T) {
	homeDir, _, gonePath := writeFixture(t)
	original, err := os.ReadFile(filepath.Join(homeDir, globalConfigFile))
	if err != nil {
		t.Fatal(err)
	}

	p := NewWithHome(homeDir)
	backup, err := p.RemoveConfigProjectEntries(fstest.MapFS{}, []string{gonePath})
	if err != nil {
		t.Fatal(err)
	}

	backupData, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(backupData) != string(original) {
		t.Errorf("backup contents differ from original")
	}
	if !strings.Contains(backup, configBackupPrefix) {
		t.Errorf("backup path %q should contain the standard prefix %q", backup, configBackupPrefix)
	}
}

// TestRemoveConfigProjectEntries_emptyKeysIsNoOp pins the
// shortcut: nothing to delete means nothing to do. The
// function should return without writing a backup or
// touching the file.
func TestRemoveConfigProjectEntries_emptyKeysIsNoOp(t *testing.T) {
	homeDir, _, _ := writeFixture(t)
	original, err := os.ReadFile(filepath.Join(homeDir, globalConfigFile))
	if err != nil {
		t.Fatal(err)
	}

	p := NewWithHome(homeDir)
	backup, err := p.RemoveConfigProjectEntries(fstest.MapFS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" {
		t.Errorf("backup path = %q, want empty for a no-op", backup)
	}

	after, err := os.ReadFile(filepath.Join(homeDir, globalConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Errorf("file changed despite no-op:\nbefore=%q\nafter=%q", original, after)
	}
}

// TestRemoveConfigProjectEntries_failsWithoutHomeDir
// mirrors the listing test. A Provider with no home dir
// cannot perform the operation.
func TestRemoveConfigProjectEntries_failsWithoutHomeDir(t *testing.T) {
	p := New()
	_, err := p.RemoveConfigProjectEntries(fstest.MapFS{}, []string{"/anything"})
	if !errors.Is(err, errMissingHomeDir) {
		t.Errorf("err = %v, want errMissingHomeDir", err)
	}
}

// TestSjsonPath_escapesDotsInDirectoryNames pins the small
// path-builder helper. Real project keys contain dots in
// directory names like "agentic.poc". Without escaping,
// sjson would try to descend into a nested object at that
// dot and fail to find the key. The test catches a
// regression by encoding a path with dots and confirming
// the dots are escaped.
func TestSjsonPath_escapesDotsInDirectoryNames(t *testing.T) {
	got := sjsonPath("projects", "/Users/x/v1.2.3/proj")
	want := `projects./Users/x/v1\.2\.3/proj`
	if got != want {
		t.Errorf("sjsonPath = %q, want %q", got, want)
	}
}

// TestPathIsDir_reportsRealDirectoryStatus pins the
// helper. We test against the test's own temp directory
// (which exists and is a directory), a missing path, and
// a path that points at a regular file.
func TestPathIsDir_reportsRealDirectoryStatus(t *testing.T) {
	dir := t.TempDir()
	if !pathIsDir(dir) {
		t.Errorf("temp dir should report as directory")
	}
	if pathIsDir(filepath.Join(dir, "no-such-thing")) {
		t.Errorf("missing path should not report as directory")
	}
	file := filepath.Join(dir, "regular.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if pathIsDir(file) {
		t.Errorf("regular file should not report as directory")
	}
}
