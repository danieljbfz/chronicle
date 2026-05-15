// Package config loads chronicle's user configuration. The file lives
// at ~/.config/chronicle/config.toml. Anything missing from that file
// falls back to the values in Defaults, and any command-line flag at
// run time overrides whatever the file or the defaults said. The rule
// of thumb is simple: defaults are what you get with no setup, the
// file is the persistent override, and flags are the per-invocation
// override.
package config

import (
	"errors"
	"io/fs"
	"os"

	"github.com/BurntSushi/toml"
)

// Provider name constants used by the config package and
// the adapter registry alike. Keeping the names in one
// place means the TOML key, the registry lookup, and any
// test fixture all reference the same string.
//
// GitHub markets several products under the umbrella name
// "Copilot." We model each as its own adapter so the
// user-facing surfaces (chronicle doctor, the providers
// map in config) tell the truth about what is on disk.
// ProviderCopilotChat is the VS Code chat extension;
// ProviderCopilotAgent is the @github/copilot-sdk runtime.
const (
	ProviderClaude       = "claude"
	ProviderCopilotChat  = "copilot-chat"
	ProviderCopilotAgent = "copilot-agent"
)

// Config is the in-memory shape of config.toml. Each nested struct
// maps to a [section] in TOML, and each field maps to a key inside
// that section through the toml:"..." struct tag at the end of the
// field declaration. The TOML decoder reads those tags through
// reflection at runtime to match the file's layout to our types.
type Config struct {
	Trash     TrashConfig               `toml:"trash"`
	UI        UIConfig                  `toml:"ui"`
	Providers map[string]ProviderConfig `toml:"providers"`
}

// TrashConfig controls how long deleted items linger in the chronicle
// trash before they can be permanently removed by the empty-trash
// command. The default of thirty days is conservative, because the
// only way to lose work to a chronicle delete is to also empty the
// trash, and a month of grace is plenty.
type TrashConfig struct {
	RetentionDays int `toml:"retention_days"`
}

// UIConfig holds the configuration for both user interfaces chronicle
// ships, namely the terminal user interface that comes later and the
// local web frontend that comes after that. We keep them under one
// section so the user has a single place to find UI settings.
type UIConfig struct {
	TUI TUIConfig `toml:"tui"`
	Web WebConfig `toml:"web"`
}

// TUIConfig collects the settings that apply only to the terminal
// frontend. Theme controls the colour scheme, FiltersDefault controls
// which content filters are on at startup, and NerdFont tells the
// renderer whether it can use Nerd Font glyphs or has to fall back to
// plain ASCII for the icons.
type TUIConfig struct {
	Theme          string   `toml:"theme"`
	FiltersDefault []string `toml:"filters_default"`
	NerdFont       string   `toml:"nerd_font"`
}

// WebConfig collects the settings that apply only to the web frontend.
// Host is the interface to bind to and we deliberately default to
// loopback only, so the server is never exposed beyond the user's own
// machine. A port of zero asks the operating system to pick any
// available port at startup, which avoids the headache of port
// conflicts. OpenBrowser controls whether chronicle pops the user's
// default browser open at the right URL when the server starts.
type WebConfig struct {
	Host        string `toml:"host"`
	Port        int    `toml:"port"`
	OpenBrowser bool   `toml:"open_browser"`
}

// ProviderConfig is the generic, provider-agnostic shape of one entry
// under [providers.<name>] in the TOML file. The same shape covers
// every adapter chronicle knows about today and every adapter that
// might land in a future release. The fields are:
//
//   - Enabled: master switch for the adapter. A user who never runs
//     Copilot can set Copilot's Enabled to false and chronicle will
//     skip the directory probes entirely.
//   - Root: the absolute filesystem path of a single-rooted adapter
//     (Claude). Empty falls back to the platform default.
//   - Roots: the list of absolute paths a multi-rooted adapter walks
//     (Copilot, with one entry per VS Code variant the user has
//     installed). Empty falls back to the platform defaults.
//   - Settings: the catch-all for adapter-specific knobs that do not
//     fit any of the above. The map's keys and values are
//     adapter-defined, so each adapter's own factory unmarshals its
//     portion into a typed local struct when it needs one. Today no
//     adapter uses this field, so the map is always empty in
//     practice.
//
// Adding a new adapter does not require any change to this type. The
// adapter's factory in adapters/all.go reads the relevant subsection
// from the Providers map, falling back to its own defaults when keys
// are absent.
type ProviderConfig struct {
	Enabled  bool                   `toml:"enabled"`
	Root     string                 `toml:"root"`
	Roots    []string               `toml:"roots"`
	Settings map[string]interface{} `toml:"settings"`
}

// Defaults returns the configuration that ships with a fresh install.
// Load merges the file contents over this baseline, so a config file
// that sets only one key still produces a fully-formed Config and
// chronicle never has to deal with zero values for fields the user
// did not mention.
//
// The Providers map is populated with one entry per adapter chronicle
// knows about by name. Adding a new adapter is one new entry here
// (with the right default Enabled value), one new factory in
// adapters/all.go, and the adapter package itself.
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
		Providers: map[string]ProviderConfig{
			ProviderClaude:       {Enabled: true},
			ProviderCopilotChat:  {Enabled: true},
			ProviderCopilotAgent: {Enabled: true},
		},
	}
}

// Load reads the config file at path and returns the result merged
// over Defaults. A missing file is not an error: the caller gets the
// default configuration and chronicle works on first run with no setup.
// A malformed file is an error, because silently ignoring a typo in
// the user's own config would be more confusing than failing fast.
//
// The errors.Is check is the standard Go way to ask "did this fail
// because the file isn't there?" The check works whether the error is
// the sentinel value directly or any error that wraps it.
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
