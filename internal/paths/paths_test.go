package paths

import (
	"path/filepath"
	"testing"
)

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
}

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
