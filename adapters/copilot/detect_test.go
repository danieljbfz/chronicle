package copilot

import (
	"os"
	"testing"
	"testing/fstest"
)

func loadCopilotFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/v3/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// TestDetect_emptyTreeReturnsUnknown confirms the harmless case:
// no Copilot data on disk at all. Detection should not return an
// error and should produce Version "unknown" so the doctor view
// can explain that nothing was found.
func TestDetect_emptyTreeReturnsUnknown(t *testing.T) {
	got, err := detectInDir(fstest.MapFS{})
	if err != nil {
		t.Fatalf("detectInDir: %v", err)
	}
	if got.Version != "unknown" {
		t.Errorf("Version = %q, want unknown", got.Version)
	}
	if got.Adapter != adapterName {
		t.Errorf("Adapter = %q, want %q", got.Adapter, adapterName)
	}
}

// TestDetect_realFixtureProducesFingerprint runs detection against
// a fake VS Code install that contains one chat session. The
// result should carry a non-empty fingerprint. Whether the version
// stays "unknown" depends on whether the small fixture happens to
// match a fingerprint already in the lookup table.
func TestDetect_realFixtureProducesFingerprint(t *testing.T) {
	fsys := fstest.MapFS{
		"workspaceStorage/abc123/chatSessions/sample.jsonl": &fstest.MapFile{
			Data: loadCopilotFixture(t, "small_session.jsonl"),
		},
	}
	got, err := detectInDir(fsys)
	if err != nil {
		t.Fatalf("detectInDir: %v", err)
	}
	if got.Fingerprint == "" {
		t.Error("Fingerprint should be set for parseable JSONL")
	}
}

// TestDetect_emptyWindowsFallback proves we still detect Copilot
// when only an empty-window chat exists. A user who only ever
// chatted in folder-less VS Code windows should still produce a
// usable fingerprint, not "unknown" because we missed the data.
func TestDetect_emptyWindowsFallback(t *testing.T) {
	fsys := fstest.MapFS{
		"globalStorage/emptyWindowChatSessions/sample.jsonl": &fstest.MapFile{
			Data: loadCopilotFixture(t, "small_session.jsonl"),
		},
	}
	got, err := detectInDir(fsys)
	if err != nil {
		t.Fatalf("detectInDir: %v", err)
	}
	if got.Fingerprint == "" {
		t.Error("Fingerprint should be set when only empty-window chats exist")
	}
}
