package claude

// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. `testing/fstest`. The standard library's in-memory filesystem for
//    tests. `fstest.MapFS` is a `map[string]*fstest.MapFile` that
//    satisfies `fs.FS` — same interface the real `os.DirFS` returns.
//    Production code calls `os.DirFS("/home/user/.claude")`; tests pass
//    an MapFS with whatever fixture content they want. The adapter
//    cannot tell the difference, which is exactly why we wired it
//    through fs.FS in the first place.
//
//    This is the Go answer to Python's `unittest.mock.patch("builtins.open")`
//    or pyfakefs — only it is built into the standard library and there
//    is nothing to patch, because the production code already speaks the
//    interface.
//
// 2. `t.Helper()`. Marks a function as a test helper. When a helper
//    calls `t.Fatalf`, the failure line in the output points at the
//    *caller* of the helper, not at the helper itself. Without this,
//    every fixture-loading failure would point at `loadFixture` instead
//    of the actual test that asked for the fixture.
//
// 3. `os.ReadFile(path)`. Small helper for "give me the whole file as
//    bytes." Equivalent to Python's `open(path, "rb").read()`. We use
//    it to load fixtures from disk into the in-memory MapFS — the test
//    file *does* read from real disk (`testdata/`), but the code under
//    test sees only the MapFS we hand it.

import (
	"os"
	"testing"
	"testing/fstest"
)

// loadFixture reads one fixture file from disk and returns its bytes.
// Marked as a helper so `t.Fatalf` blames the calling test, not this
// function (concept #2 above).
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/v1_0/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestDetect_emptyTreeReturnsUnknown(t *testing.T) {
	// An empty MapFS is the simplest possible "user has no Claude data
	// at all" scenario. Detect should return "unknown" with no error.
	fsys := fstest.MapFS{}
	got, err := detectInDir(fsys)
	if err != nil {
		t.Fatalf("detectInDir: %v", err)
	}
	if got.Version != "unknown" {
		t.Errorf("Version = %q, want %q", got.Version, "unknown")
	}
	if got.Adapter != "claude" {
		t.Errorf("Adapter = %q, want %q", got.Adapter, "claude")
	}
}

func TestDetect_realFixtureProducesFingerprint(t *testing.T) {
	// Lay out a fake ~/.claude with one project and one session inside.
	// The MapFS path looks exactly like a real Claude install.
	fsys := fstest.MapFS{
		"projects/-Users-test-proj/small.jsonl": &fstest.MapFile{
			Data: loadFixture(t, "small_session.jsonl"),
		},
	}
	got, err := detectInDir(fsys)
	if err != nil {
		t.Fatalf("detectInDir: %v", err)
	}
	if got.Fingerprint == "" {
		t.Error("Fingerprint should be set for parseable JSONL")
	}
	// The version stays "unknown" until Task 20 adds the captured
	// fingerprint to knownFingerprints. That is intentional: we want
	// the very first run on a contributor's machine to produce the
	// fingerprint as evidence, not as guesswork.
	if got.Version != "unknown" {
		t.Logf("Version = %q (will become claude-1.0 once Task 20 adds the fingerprint)", got.Version)
	}
}

func TestDetect_garbageMixedWithJSONStillProducesFingerprint(t *testing.T) {
	// One garbage line followed by one valid record. The resilience
	// contract says we skip garbage and keep going; the fingerprint
	// should reflect the one record we managed to parse.
	fsys := fstest.MapFS{
		"projects/p/s.jsonl": &fstest.MapFile{Data: []byte("not json\n{\"type\":\"user\"}\n")},
	}
	got, err := detectInDir(fsys)
	if err != nil {
		t.Fatalf("detectInDir: %v", err)
	}
	if got.Fingerprint == "" {
		t.Error("Fingerprint should still be computed when one line parses")
	}
}
