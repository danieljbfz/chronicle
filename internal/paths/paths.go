// Package paths is the one place chronicle figures out where the
// user's config and data files live on disk. The rest of the codebase
// never builds these paths by hand, because hand-built paths would
// defeat the override mechanism the test suite relies on. The package
// lives under the internal directory, which is a Go convention the
// compiler enforces: anything inside an internal directory is only
// importable by code in the same module, so these helpers cannot leak
// out if someone ever imports chronicle as a library.
package paths

import (
	"os"
	"path/filepath"
)

// Locations holds the resolved filesystem paths for the running
// process. Construction happens once at startup, inside composition.New,
// and the result travels down to whichever code needs it. The struct is
// small, so we pass it by value everywhere and never deal with pointers.
type Locations struct {
	ConfigDir  string
	ConfigFile string
	TrashDir   string
	ReportsDir string
	ClaudeRoot string
}

// Resolve returns the default Locations for the current user. The home
// directory comes from the standard library's os.UserHomeDir, unless
// the CHRONICLE_HOME environment variable is set, in which case we use
// that instead. The override exists for the test suite, which sets
// CHRONICLE_HOME to a fresh temporary directory at the start of every
// test that touches anything under the chronicle config path. The
// production code calls Resolve exactly the same way the tests do, and
// the only difference is whether the environment variable is set.
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

// homeDir is the seam through which the test suite redirects the entire
// path namespace. The function is unexported because no caller outside
// this package has any business resolving the home directory directly.
func homeDir() (string, error) {
	if override := os.Getenv("CHRONICLE_HOME"); override != "" {
		return override, nil
	}
	return os.UserHomeDir()
}
