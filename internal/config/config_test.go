package config

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestLoad_malformedTOMLReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.toml")
	if err := os.WriteFile(path, []byte("this is not = valid = toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("Load should return an error on malformed TOML")
	}
}
