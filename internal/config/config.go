// Package config loads and writes chronicle's user configuration. The file
// lives at ~/.config/chronicle/config.toml. Missing fields fall back to
// Defaults. Every command-line flag overrides the config for that
// invocation only.
package config

import (
	"errors"
	"io/fs"
	"os"

	"github.com/BurntSushi/toml"
)

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

// Defaults returns the configuration shipped with a fresh install.
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

// Load reads the config file at path and returns it merged over Defaults.
// A missing file is not an error — the caller gets Defaults.
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
