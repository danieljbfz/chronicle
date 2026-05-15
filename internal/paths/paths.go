// Package paths centralizes every filesystem location chronicle reads or
// writes outside provider data. Callers never construct these paths by
// hand — that keeps tests deterministic when we override the home dir.
package paths

import (
	"os"
	"path/filepath"
)

// Locations holds the resolved paths for the running process. Constructed
// once at startup; passed down where needed.
type Locations struct {
	ConfigDir  string // ~/.config/chronicle
	ConfigFile string // ~/.config/chronicle/config.toml
	TrashDir   string // ~/.config/chronicle/trash
	ReportsDir string // ~/.config/chronicle/format-reports
	ClaudeRoot string // ~/.claude
}

// Resolve returns the default Locations for the current user. Override the
// home directory by setting the CHRONICLE_HOME environment variable, which
// tests use to redirect every path under a temp dir.
func Resolve() (Locations, error) {
	home, err := homeDir()
	if err != nil {
		return Locations{}, err
	}
	config := filepath.Join(home, ".config", "chronicle")
	return Locations{
		ConfigDir:  config,
		ConfigFile: filepath.Join(config, "config.toml"),
		TrashDir:   filepath.Join(config, "trash"),
		ReportsDir: filepath.Join(config, "format-reports"),
		ClaudeRoot: filepath.Join(home, ".claude"),
	}, nil
}

func homeDir() (string, error) {
	if override := os.Getenv("CHRONICLE_HOME"); override != "" {
		return override, nil
	}
	return os.UserHomeDir()
}
