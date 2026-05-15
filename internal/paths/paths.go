// Package paths centralizes every filesystem location chronicle reads or
// writes outside provider data. Callers never construct these paths by
// hand — that keeps tests deterministic when we override the home dir.
//
// The package lives under `internal/` for a reason: Go's compiler enforces
// that anything inside an `internal/` directory may only be imported by
// code inside the same module. So `internal/paths` is unreachable to
// outsiders even if someone vendors chronicle into their project. Use
// `internal/` whenever you want to keep an implementation detail private.
//
// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. THE `internal/` DIRECTORY. Explained above. Compiler-enforced
//    privacy, no opt-in needed.
//
// 2. THE `path/filepath` PACKAGE. The standard library's "join paths
//    correctly for this OS" helper. On macOS/Linux it uses `/`, on Windows
//    `\`. Always use `filepath.Join` rather than concatenating strings —
//    cross-platform correctness for free.
//
// 3. ENVIRONMENT-VARIABLE OVERRIDE FOR TESTS. `os.Getenv("CHRONICLE_HOME")`
//    lets tests redirect every path under a temp directory by setting
//    one env var. The production code calls `Resolve()` exactly the same
//    way; only the env is different. This keeps the production path
//    simple and tests deterministic. The standard library's `t.Setenv`
//    in tests sets and auto-restores the env var.

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

// Resolve returns the default Locations for the current user. Override
// the home directory by setting the CHRONICLE_HOME environment variable,
// which tests use to redirect every path under a temp dir.
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

// homeDir is package-private (lowercase name). It is the seam tests use
// to redirect the entire path namespace.
func homeDir() (string, error) {
	if override := os.Getenv("CHRONICLE_HOME"); override != "" {
		return override, nil
	}
	return os.UserHomeDir()
}
