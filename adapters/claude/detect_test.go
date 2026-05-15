package claude

import (
	"os"
	"testing"
	"testing/fstest"
)

// loadFixture reads one fixture file from the testdata directory and
// returns its bytes. We mark it as a test helper so when t.Fatalf
// fires inside the helper, the failure line in the test output
// points at the calling test, not at this function. Without
// t.Helper(), every fixture-loading failure would blame the helper
// instead of the test that asked for the fixture.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/v1_0/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// TestDetect_emptyTreeReturnsUnknown is the simplest possible
// scenario: the user has no Claude data at all. Detect should return
// a StorageVersion with Version equal to "unknown" and no error,
// because chronicle should still load and the doctor view should be
// able to explain that nothing was found.
func TestDetect_emptyTreeReturnsUnknown(t *testing.T) {
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

// TestDetect_realFixtureProducesFingerprint runs detection against
// a fake ~/.claude built in memory from one of our fixture files.
// The result should carry a non-empty fingerprint. The version
// will probably stay "unknown" because the small fixture has a
// different shape from the real Claude installs we have already
// fingerprinted into knownFingerprints. That is fine here. The
// test only checks that detection produced a fingerprint, not
// which one.
func TestDetect_realFixtureProducesFingerprint(t *testing.T) {
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
	if got.Version != "unknown" {
		t.Logf("Version = %q (the fixture happens to match a known fingerprint)", got.Version)
	}
}

// TestDetect_garbageMixedWithJSONStillProducesFingerprint is the
// resilience-side test. One garbage line followed by one valid record
// should produce a fingerprint that reflects the one record we
// managed to parse, not a crash. The contract says we tolerate bad
// lines and keep going.
func TestDetect_garbageMixedWithJSONStillProducesFingerprint(t *testing.T) {
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
