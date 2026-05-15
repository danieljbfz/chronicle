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
	"github.com/danieljbfz/chronicle/adapters/copilotagent"
	"github.com/danieljbfz/chronicle/adapters/copilotchat"
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

// Factory builds zero or more Entry values from the user's config
// and the resolved filesystem paths. Most factories return a single
// Entry. Some return several (the Copilot factory returns one Entry
// per detected install, because the user might have both VS Code
// and VS Code Insiders). A factory that returns nil means the
// provider is disabled or has no data on this machine.
type Factory func(config.Config, paths.Locations) []Entry

// All returns every registered provider factory. This is the single
// place chronicle looks to discover which providers it knows about.
// Adding a new provider is one new line below.
func All() []Factory {
	return []Factory{
		claudeFactory,
		copilotChatFactory,
		copilotAgentFactory,
		// Future plans add cursorFactory, antigravityFactory,
		// and so on. Each new line is one import above and
		// one entry here, with no other change to the rest
		// of chronicle.
	}
}

// claudeFactory builds the Claude adapter from the user's config.
// It returns nil when the user has disabled the Claude provider in
// their config file. When no explicit root is set in the config, it
// falls back to the default ~/.claude location resolved by the
// paths package.
func claudeFactory(settings config.Config, locations paths.Locations) []Entry {
	cfg := settings.Providers[config.ProviderClaude]
	if !cfg.Enabled {
		return nil
	}
	root := cfg.Root
	if root == "" {
		root = locations.ClaudeRoot
	}
	return []Entry{{
		Provider: claude.NewWithHome(locations.HomeDir),
		Root:     root,
		FS:       os.DirFS(root),
	}}
}

// copilotAgentFactory builds one Entry for the GitHub
// Copilot agent runtime (the @github/copilot-sdk
// LocalSessionManager) when its session-state directory
// exists on disk. The runtime persists at ~/.copilot/ on
// every platform it supports today, so the factory
// produces at most one Entry per chronicle install.
//
// This is the second of two Copilot products chronicle
// models. The other is the Copilot Chat extension whose
// data lives under VS Code's workspaceStorage. The two
// have non-overlapping data on disk, so each gets its own
// Entry and the user sees both in chronicle doctor.
func copilotAgentFactory(settings config.Config, locations paths.Locations) []Entry {
	cfg := settings.Providers[config.ProviderCopilotAgent]
	if !cfg.Enabled {
		return nil
	}
	root := cfg.Root
	if root == "" {
		root = locations.CopilotAgentRoot
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	return []Entry{{
		Provider: copilotagent.New(),
		Root:     root,
		FS:       os.DirFS(root),
	}}
}

// copilotChatFactory builds one Entry per Copilot Chat root
// that exists on disk. The user's config provides the
// candidate roots (defaulting to the macOS VS Code and VS
// Code Insiders locations), and we silently skip any root
// that is missing. The user might have only VS Code
// installed, or only VS Code Insiders, or both. Each
// surviving root gets its own Provider value so each one
// keeps its own cached storage version.
//
// Copilot Chat is one of two distinct GitHub Copilot
// products chronicle models. The other is the agent
// runtime (~/.copilot/) handled by the copilotagent
// adapter. Both share the user-facing brand "Copilot" but
// produce non-overlapping data with different file
// formats, so each gets its own adapter.
func copilotChatFactory(settings config.Config, locations paths.Locations) []Entry {
	cfg := settings.Providers[config.ProviderCopilotChat]
	if !cfg.Enabled {
		return nil
	}
	roots := cfg.Roots
	if len(roots) == 0 {
		roots = locations.CopilotChatRoots
	}

	var entries []Entry
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		entries = append(entries, Entry{
			Provider: copilotchat.New(),
			Root:     root,
			FS:       os.DirFS(root),
		})
	}
	return entries
}
