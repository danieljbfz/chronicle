// Package composition is the only layer in chronicle that talks to
// the real filesystem. It builds the providers from the registry,
// hands each one a fs.FS pointed at its data directory, and exposes
// a small set of methods the entrypoints call into. The CLI is the
// only entrypoint today. Future entrypoints (a terminal UI, a local
// web frontend) will use the same methods.
//
// The split between composition and the entrypoints is on purpose.
// The CLI does not know about the Claude adapter or any other
// adapter. It calls App.ListSessionsAll and gets back a list. The
// entrypoint stays thin and the layering stays clean.
package composition

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"golang.org/x/sync/errgroup"

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
//
// cache holds the persistent session-summary cache, loaded lazily on
// the first listing and shared across every later one. cacheOnce guards
// that one-time load.
type App struct {
	settings  config.Config
	locations paths.Locations
	providers []*providerEntry
	cache     *summaryCache
	cacheOnce sync.Once
}

// New builds an App. It resolves the filesystem paths, loads the
// user's config, walks the adapter registry to build one provider
// per enabled tool, and runs Detect on each provider so the doctor
// view has results to show.
//
// Detect runs in parallel across all providers. Each Detect call
// reads one small file from disk, and we have no reason to wait
// for the slowest one before starting the next. With two
// providers (Claude and Copilot) the speedup is barely visible.
// With ten providers, the savings start to matter, and the code
// shape stays the same.
//
// We use errgroup, the standard "fan out N tasks, wait for all,
// collect first error" helper from golang.org/x/sync. It cleans
// up the error and synchronisation handling that a hand-rolled
// sync.WaitGroup would otherwise need.
//
// Detect failures get classified the same way they did when this
// ran serially. A provider whose data directory is missing (e.g.,
// the user has not installed that tool, or has never run it)
// reports fs.ErrNotExist. We treat that as "this provider is just
// not active right now" and move on. Any other Detect error is
// real, and the first one we hit aborts New.
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
		for _, entry := range factory(settings, locations) {
			a.providers = append(a.providers, &providerEntry{
				Provider: entry.Provider,
				Root:     entry.Root,
				FS:       entry.FS,
			})
		}
	}

	if err := a.detectAll(); err != nil {
		return nil, err
	}

	return a, nil
}

// detectAll fans out Detect across every registered provider in
// parallel and waits for all of them to finish before returning.
// We split it out from New so the parallel logic has one clear
// home and so tests can call it without rebuilding the rest of
// the App.
func (a *App) detectAll() error {
	group := new(errgroup.Group)
	for _, p := range a.providers {
		group.Go(func() error {
			sv, err := p.Provider.Detect(p.FS)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			// Each provider entry is its own pointer, so writing
			// to p.Version from a goroutine does not race with
			// any other goroutine. The race detector confirms
			// this in `make test`.
			p.Version = sv
			return nil
		})
	}
	return group.Wait()
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
	defer a.flushSummaryCache()
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
			sessions, err := a.summariesForProject(p, proj.ID)
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
// command and the trash command use this to show or operate on
// chronicle's own directories.
func (a *App) Locations() paths.Locations { return a.locations }

// SettingsTOML returns the resolved config rendered as TOML, the
// same format the user's config file uses on disk. The output is
// what `chronicle config show` prints. Defaults plus any
// file-level overrides are merged before rendering, so the
// reader sees the actual values chronicle is using right now,
// not just what they wrote in their file.
//
// We render through the same TOML library that loads the file,
// so the output round-trips: the user can pipe it back into
// their config file and get the same Config back. The library
// preserves struct order, which keeps the output stable across
// invocations.
func (a *App) SettingsTOML() (string, error) {
	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(a.settings); err != nil {
		return "", fmt.Errorf("config show: %w", err)
	}
	return buf.String(), nil
}
