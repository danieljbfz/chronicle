// Package config loads and writes chronicle's user configuration. The
// file lives at ~/.config/chronicle/config.toml. Missing fields fall back
// to Defaults(). Every command-line flag overrides the config for that
// invocation only.
//
// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. THIRD-PARTY IMPORTS via `go get`. `github.com/BurntSushi/toml` was
//    added to go.mod with `go get github.com/BurntSushi/toml@latest`.
//    Go fetches the package, records the exact version in go.mod, and
//    a checksum in go.sum. Both files get committed.
//
// 2. STRUCT TAGS. The backticks at the end of each field — `toml:"name"`
//    — are *struct tags*. They are strings the standard library and
//    third-party libraries read via reflection at runtime. Here, the TOML
//    decoder reads `toml:"retention_days"` and learns "when decoding
//    TOML, the value at key `retention_days` goes into the
//    RetentionDays field." JSON has its own tag: `json:"retention_days"`.
//    YAML, env, sql columns — every library that maps between text and
//    Go structs uses tags.
//
// 3. ERROR WRAPPING vs ERROR INSPECTION. The check
//        if errors.Is(err, fs.ErrNotExist) { ... }
//    is the canonical way to test for a specific kind of error in Go.
//    `errors.Is` walks any chain of wrapped errors, so it works whether
//    the error is the sentinel directly or a wrapped descendant of it.
//    The matching `errors.As(err, &target)` extracts a typed error
//    value, useful when you need its fields.
//
// 4. THE `os.ReadFile` HELPER. Reads an entire file into memory in one
//    call. Fine for small files like config (a few KB max). For large
//    files we stream via `bufio.Scanner` or `io.Copy`.

package config

import (
	"errors"
	"io/fs"
	"os"

	"github.com/BurntSushi/toml"
)

// Config is the in-memory shape of config.toml. Each nested struct maps
// to a `[section]` in TOML; each field maps to a key inside that section
// via the `toml:"..."` tag.
type Config struct {
	Trash     TrashConfig     `toml:"trash"`
	UI        UIConfig        `toml:"ui"`
	Providers ProvidersConfig `toml:"providers"`
}

type TrashConfig struct {
	RetentionDays int `toml:"retention_days"`
}

type UIConfig struct {
	TUI TUIConfig `toml:"tui"`
	Web WebConfig `toml:"web"`
}

type TUIConfig struct {
	Theme          string   `toml:"theme"`
	FiltersDefault []string `toml:"filters_default"`
	NerdFont       string   `toml:"nerd_font"`
}

type WebConfig struct {
	Host        string `toml:"host"`
	Port        int    `toml:"port"`
	OpenBrowser bool   `toml:"open_browser"`
}

type ProvidersConfig struct {
	Claude  ClaudeConfig  `toml:"claude"`
	Copilot CopilotConfig `toml:"copilot"`
}

type ClaudeConfig struct {
	Enabled bool   `toml:"enabled"`
	Root    string `toml:"root"`
}

type CopilotConfig struct {
	Enabled                 bool     `toml:"enabled"`
	Roots                   []string `toml:"roots"`
	RefuseWhenVSCodeRunning bool     `toml:"refuse_when_vscode_running"`
}

// Defaults returns the configuration shipped with a fresh install. Load()
// merges file values over this baseline, so a config file that sets only
// one key still produces a fully-formed Config.
func Defaults() Config {
	return Config{
		Trash: TrashConfig{RetentionDays: 30},
		UI: UIConfig{
			TUI: TUIConfig{
				Theme:          "auto",
				FiltersDefault: []string{"tools", "meta"},
				NerdFont:       "auto",
			},
			Web: WebConfig{
				Host:        "127.0.0.1",
				Port:        0,
				OpenBrowser: true,
			},
		},
		Providers: ProvidersConfig{
			Claude: ClaudeConfig{
				Enabled: true,
			},
			Copilot: CopilotConfig{
				Enabled:                 true,
				RefuseWhenVSCodeRunning: true,
			},
		},
	}
}

// Load reads the config file at path and returns it merged over
// Defaults. A missing file is not an error — the caller gets Defaults.
// A malformed file is an error.
//
// The "missing file = defaults" rule lets us ship a binary that works on
// first run without forcing the user to write a config first. The
// `errors.Is(err, fs.ErrNotExist)` check is the canonical way to ask
// "did this fail because the file isn't there?" — see concept #3 above.
func Load(path string) (Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, err
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
