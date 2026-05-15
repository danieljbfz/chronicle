// Package composition is the only layer in chronicle that talks to
// the real filesystem. It builds the providers from the registry,
// hands each one a fs.FS pointed at its data directory, and exposes
// a small set of methods the entrypoints (the CLI today, the TUI
// and web frontends in later plans) call into.
//
// The split between composition and the entrypoints is on purpose.
// The CLI does not know about the Claude adapter or any other
// adapter. It calls App.ListSessionsAll and gets back a list. The
// entrypoint stays thin and the layering stays clean.
package composition

import (
	"errors"
	"io/fs"

	"github.com/danieljbfz/chronicle/adapters"
	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/internal/config"
	"github.com/danieljbfz/chronicle/internal/paths"
)

// providerEntry is the internal version of adapters.Entry. We add
// the cached StorageVersion that Detect returned, so the rest of
// composition can read it without calling Detect again.
type providerEntry struct {
	Provider contracts.Provider
	Root     string
	FS       fs.FS
	Version  contracts.StorageVersion
}

// App is the wired-up composition the entrypoints use. The
// expected lifecycle is one App per chronicle process. Entrypoints
// build it once at startup with New, then call its read methods.
type App struct {
	settings  config.Config
	locations paths.Locations
	providers []*providerEntry
}

// New builds an App. It resolves the filesystem paths, loads the
// user's config, walks the adapter registry to build one provider
// per enabled tool, and runs Detect on each provider so the doctor
// view has results to show.
//
// Detect failures get a small note. A provider whose data
// directory is missing (e.g., the user has not installed that
// tool, or has never run it) reports fs.ErrNotExist. We treat
// that as "this provider is just not active right now" and move
// on. Other Detect errors are real, and we surface them.
func New() (*App, error) {
	locations, err := paths.Resolve()
	if err != nil {
		return nil, err
	}
	settings, err := config.Load(locations.ConfigFile)
	if err != nil {
		return nil, err
	}

	a := &App{settings: settings, locations: locations}

	for _, factory := range adapters.All() {
		entry, ok := factory(settings, locations)
		if !ok {
			continue
		}
		a.providers = append(a.providers, &providerEntry{
			Provider: entry.Provider,
			Root:     entry.Root,
			FS:       entry.FS,
		})
	}

	for _, p := range a.providers {
		sv, err := p.Provider.Detect(p.FS)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		p.Version = sv
	}

	return a, nil
}

// NewForTest builds an App from fakes. Production code should use
// New, which reads real config and resolves real paths. Tests use
// this constructor to skip the filesystem entirely and pass a hand-
// built provider plus a hand-built fs.FS.
func NewForTest(providers []contracts.Provider, roots []fs.FS) *App {
	a := &App{}
	for i, p := range providers {
		var fsys fs.FS
		if i < len(roots) {
			fsys = roots[i]
		}
		a.providers = append(a.providers, &providerEntry{Provider: p, FS: fsys})
	}
	return a
}

// ProjectListing is one row of the cross-provider project list. It
// pairs a Project with the provider name that owns it and the
// storage version that produced it.
type ProjectListing struct {
	Provider string
	Project  contracts.Project
	Source   contracts.StorageVersion
}

// ListProjects returns every project across every detected
// provider. The list is meant for the cross-provider "show me
// everything" view. The order is provider-by-provider in
// registration order, with the projects inside each provider
// already sorted by display name.
func (a *App) ListProjects() ([]ProjectListing, error) {
	var out []ProjectListing
	for _, p := range a.providers {
		projects, err := p.Provider.ListProjects(p.FS)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, proj := range projects {
			out = append(out, ProjectListing{
				Provider: p.Provider.Name(),
				Project:  proj,
				Source:   p.Version,
			})
		}
	}
	return out, nil
}

// SessionListing is one row of the cross-provider session list. It
// pairs a SessionSummary with the provider name that owns it.
type SessionListing struct {
	Provider string
	Summary  contracts.SessionSummary
}

// ListSessionsAll returns every session across every project across
// every detected provider. Pass providerName to limit the result to
// one tool, or the empty string to get everything. The CLI list
// command is the main caller of this method.
func (a *App) ListSessionsAll(providerName string) ([]SessionListing, error) {
	var out []SessionListing
	for _, p := range a.providers {
		if providerName != "" && p.Provider.Name() != providerName {
			continue
		}
		projects, err := p.Provider.ListProjects(p.FS)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, proj := range projects {
			sessions, err := p.Provider.ListSessions(p.FS, proj.ID)
			if err != nil {
				return nil, err
			}
			for _, s := range sessions {
				out = append(out, SessionListing{Provider: p.Provider.Name(), Summary: s})
			}
		}
	}
	return out, nil
}

// ReadSession looks up one session by identifier across every
// provider and returns the parsed Conversation. It returns
// fs.ErrNotExist when no provider knows the identifier. The CLI
// export and copy commands use this to fetch the session the user
// asked for, without making the user say which provider it lives
// in.
func (a *App) ReadSession(id contracts.SessionID) (contracts.Conversation, error) {
	for _, p := range a.providers {
		c, err := p.Provider.ReadSession(p.FS, id)
		if err == nil {
			return c, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return contracts.Conversation{}, err
		}
	}
	return contracts.Conversation{}, fs.ErrNotExist
}

// Settings returns the user's resolved config. Callers that need
// to override a config value with a command-line flag read the
// config here, apply the override to their own local copy, and use
// that local copy for the rest of the call. Composition never
// mutates the shared config.
func (a *App) Settings() config.Config { return a.settings }

// Locations returns the resolved filesystem locations. The doctor
// command and the trash command (in a later plan) use this to show
// or operate on chronicle's own directories.
func (a *App) Locations() paths.Locations { return a.locations }
