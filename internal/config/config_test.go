package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_missingFileReturnsDefaults proves the on-first-run
// behaviour: chronicle should work without any config file existing
// at all. The test asks for a path that does not exist and checks
// that the result matches what Defaults would produce.
func TestLoad_missingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("Load(missing): %v", err)
	}
	if cfg.Trash.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30", cfg.Trash.RetentionDays)
	}
	if !cfg.Providers.Claude.Enabled {
		t.Error("Claude should be enabled by default")
	}
}

// TestLoad_overridesDefaults proves the merge behaviour: a file that
// only sets some keys still produces a fully-formed Config, with the
// keys the file mentions taking the file's values and the keys the
// file does not mention keeping their default values. This matters
// because chronicle should never force the user to write out the
// entire schema just to change one option.
func TestLoad_overridesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `
[trash]
retention_days = 7

[providers.claude]
enabled = false
root    = "/some/where"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Trash.RetentionDays != 7 {
		t.Errorf("RetentionDays = %d, want 7", cfg.Trash.RetentionDays)
	}
	if cfg.Providers.Claude.Enabled {
		t.Error("Claude should be disabled per file")
	}
	if cfg.Providers.Claude.Root != "/some/where" {
		t.Errorf("Root = %q, want %q", cfg.Providers.Claude.Root, "/some/where")
	}
	if cfg.Providers.Copilot.Enabled != true {
		t.Error("Copilot should remain enabled (default)")
	}
}

// TestLoad_malformedTOMLReturnsError proves we fail loudly on a typo
// in the user's config file rather than silently falling back to
// defaults. Falling back silently would hide bugs in the user's own
// configuration and produce surprising behaviour at runtime.
func TestLoad_malformedTOMLReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.toml")
	if err := os.WriteFile(path, []byte("this is not = valid = toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("Load should return an error on malformed TOML")
	}
}
