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
	if !cfg.Providers[ProviderClaude].Enabled {
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
	if cfg.Providers[ProviderClaude].Enabled {
		t.Error("Claude should be disabled per file")
	}
	if cfg.Providers[ProviderClaude].Root != "/some/where" {
		t.Errorf("Root = %q, want %q", cfg.Providers[ProviderClaude].Root, "/some/where")
	}
	if !cfg.Providers[ProviderCopilotChat].Enabled {
		t.Error("Copilot should remain enabled (default)")
	}
}

// TestLoad_unknownProviderRoundsTrip proves the map shape
// is genuinely provider-agnostic. A config file that
// declares a provider chronicle does not yet ship (say, a
// "cursor" or "antigravity" subsection) is parsed without
// error and the entry is available in the map for any
// future adapter factory to read. The point is that the
// config layer never has to learn a provider name to load
// its config.
func TestLoad_unknownProviderRoundsTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `
[providers.cursor]
enabled = true
root    = "/Users/x/.cursor"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cursor := cfg.Providers["cursor"]
	if !cursor.Enabled {
		t.Error("future provider 'cursor' should be enabled per file")
	}
	if cursor.Root != "/Users/x/.cursor" {
		t.Errorf("Root = %q, want %q", cursor.Root, "/Users/x/.cursor")
	}
}

// TestLoad_tuiGlamourStyleDefault confirms the default style the
// transcript reader uses when the user has not set one. The
// chronicle binary's main package treats this value as the
// fallback for any unknown value the user might type into the
// glamour_style key, so a regression in the default would
// silently flip every transcript over to a different look.
func TestLoad_tuiGlamourStyleDefault(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("Load(missing): %v", err)
	}
	if cfg.UI.TUI.GlamourStyle != "dark" {
		t.Errorf("default glamour_style = %q, want %q", cfg.UI.TUI.GlamourStyle, "dark")
	}
}

// TestLoad_tuiGlamourStyleOverride confirms the round-trip from
// disk into the field the TUI reads. The chronicle binary's
// main package consumes this value through cfg.UI.TUI.GlamourStyle,
// so the test pins the path the live runtime depends on.
func TestLoad_tuiGlamourStyleOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `
[ui.tui]
glamour_style = "tokyo-night"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.UI.TUI.GlamourStyle != "tokyo-night" {
		t.Errorf("glamour_style = %q, want %q", cfg.UI.TUI.GlamourStyle, "tokyo-night")
	}
}

// TestLoad_malformedTOMLReturnsError proves we fail loudly on a typo
// in the user's config file. Falling back silently to defaults
// would hide bugs in the user's own configuration and produce
// surprising behaviour at runtime.
func TestLoad_malformedTOMLReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.toml")
	if err := os.WriteFile(path, []byte("this is not = valid = toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("Load should return an error on malformed TOML")
	}
}
