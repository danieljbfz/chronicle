// Package adapters wires the per-tool provider packages into one
// list the application core can iterate. Adding a new provider to
// chronicle is a matter of writing a new package under adapters/
// and adding one entry to the list returned by All. Nothing else
// in the codebase has to change.
//
// Why does the registry live one folder above the per-tool
// packages? Because the per-tool packages should not know anything
// about config, paths, or the os.DirFS wiring. The claude package
// knows about ~/.claude and nothing else. The wiring lives here, in
// adapters/all.go, and uses the per-tool packages as building
// blocks.
package adapters

import (
	"io/fs"
	"os"

	"github.com/danieljbfz/chronicle/adapters/claude"
	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/internal/config"
	"github.com/danieljbfz/chronicle/internal/paths"
)

// Entry is one wired-up provider, ready for the application core to
// use. The Provider field is the adapter itself. The Root field is
// the absolute filesystem path the adapter is reading from, kept
// for the doctor view. The FS field is what the adapter actually
// uses for I/O, normally an os.DirFS pointed at Root.
type Entry struct {
	Provider contracts.Provider
	Root     string
	FS       fs.FS
}

// Factory builds an Entry from the user's config and the resolved
// filesystem paths. It returns ok=false when the provider is
// disabled in config or when its data root is missing on disk. The
// application core skips factories that return ok=false, so the
// rest of chronicle never has to check whether a provider is
// available.
type Factory func(config.Config, paths.Locations) (Entry, bool)

// All returns every registered provider factory. This is the single
// place chronicle looks to discover which providers it knows about.
// Adding a new provider is one new line below.
func All() []Factory {
	return []Factory{
		claudeFactory,
		// Future plans add copilotFactory, cursorFactory,
		// antigravityFactory, and so on. Each new line is one
		// import above and one entry here, with no other change to
		// the rest of chronicle.
	}
}

// claudeFactory builds the Claude adapter from the user's config.
// It returns ok=false when the user has disabled the Claude
// provider in their config file. When no explicit root is set in
// the config, it falls back to the default ~/.claude location
// resolved by the paths package.
func claudeFactory(settings config.Config, locations paths.Locations) (Entry, bool) {
	if !settings.Providers.Claude.Enabled {
		return Entry{}, false
	}
	root := settings.Providers.Claude.Root
	if root == "" {
		root = locations.ClaudeRoot
	}
	return Entry{
		Provider: claude.New(),
		Root:     root,
		FS:       os.DirFS(root),
	}, true
}
