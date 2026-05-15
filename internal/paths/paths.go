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
	"runtime"
)

// Locations holds the resolved filesystem paths for the running
// process. Construction happens once at startup, inside composition.New,
// and the result travels down to whichever code needs it. The struct is
// small, so we pass it by value everywhere and never deal with pointers.
//
// CopilotRoots is a slice because Copilot data lives in several
// places at once. The default list covers VS Code and VS Code
// Insiders. Users with Cursor or other VS Code forks can extend the
// list through their config file.
type Locations struct {
	ConfigDir    string
	ConfigFile   string
	TrashDir     string
	ReportsDir   string
	ClaudeRoot   string
	CopilotRoots []string
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
		ConfigDir:    config,
		ConfigFile:   filepath.Join(config, "config.toml"),
		TrashDir:     filepath.Join(config, "trash"),
		ReportsDir:   filepath.Join(config, "format-reports"),
		ClaudeRoot:   filepath.Join(home, ".claude"),
		CopilotRoots: defaultCopilotRoots(home),
	}, nil
}

// defaultCopilotRoots returns the standard list of paths where
// VS Code and its sibling installs keep their Copilot data on the
// current operating system. We list both regular VS Code and
// VS Code Insiders. Users with other VS Code forks (Cursor,
// VSCodium, and so on) can extend the list through the config
// file.
//
// The path layout follows VS Code's own conventions, which are
// well documented and the same on every machine of a given
// operating system:
//
//	macOS:    ~/Library/Application Support/Code/User
//	Linux:    ~/.config/Code/User
//	Windows:  %APPDATA%/Code/User    (typically ~/AppData/Roaming/Code/User)
//
// We use os.UserConfigDir to find the right base directory on
// Linux and Windows, because that helper already knows about
// XDG_CONFIG_HOME and the Windows %APPDATA% variable.
func defaultCopilotRoots(home string) []string {
	switch runtime.GOOS {
	case "darwin":
		base := filepath.Join(home, "Library", "Application Support")
		return []string{
			filepath.Join(base, "Code", "User"),
			filepath.Join(base, "Code - Insiders", "User"),
		}
	default:
		// Linux, Windows, and any other OS Go supports. We resolve
		// the per-user config directory through the standard
		// library so we get the right answer for each platform's
		// conventions.
		base, err := os.UserConfigDir()
		if err != nil {
			return nil
		}
		return []string{
			filepath.Join(base, "Code", "User"),
			filepath.Join(base, "Code - Insiders", "User"),
		}
	}
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
