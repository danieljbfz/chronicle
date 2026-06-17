package paths

import (
	"path/filepath"
	"testing"
)

// TestResolve_usesEnvOverride confirms that the CHRONICLE_HOME env
// override redirects every resolved path. The whole point of having
// the override is to make the test suite deterministic, so this test
// is the most important one in the file: if it ever fails, every
// downstream test that touches the filesystem starts fighting with
// the contributor's actual home directory.
func TestResolve_usesEnvOverride(t *testing.T) {
	t.Setenv("CHRONICLE_HOME", "/tmp/fake-home")
	loc, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve(): %v", err)
	}
	want := filepath.Join("/tmp/fake-home", ".config", "chronicle", "config.toml")
	if loc.ConfigFile != want {
		t.Errorf("ConfigFile = %q, want %q", loc.ConfigFile, want)
	}
	if loc.ClaudeRoot != "/tmp/fake-home/.claude" {
		t.Errorf("ClaudeRoot = %q, want %q", loc.ClaudeRoot, "/tmp/fake-home/.claude")
	}
	wantCache := filepath.Join("/tmp/fake-home", ".cache", "chronicle")
	if loc.CacheDir != wantCache {
		t.Errorf("CacheDir = %q, want %q", loc.CacheDir, wantCache)
	}
}

// TestResolve_realHomeWhenNoOverride confirms the production fallback:
// when CHRONICLE_HOME is empty, Resolve falls back to the real home
// directory and produces a non-empty ClaudeRoot. We cannot assert the
// exact value because it depends on whoever runs the tests, but a
// non-empty result is enough to prove the fallback works.
func TestResolve_realHomeWhenNoOverride(t *testing.T) {
	t.Setenv("CHRONICLE_HOME", "")
	loc, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve(): %v", err)
	}
	if loc.ClaudeRoot == "" {
		t.Error("ClaudeRoot should be set")
	}
}
