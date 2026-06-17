package composition

import (
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/steps"
)

// BulkExportOptions controls App.BulkExport. Provider scopes
// the lookup to one adapter when several happen to know the
// same project identifier (a synthetic case today, but
// possible the moment a future adapter ships). The boolean
// filters mirror the single-session export so the bulk path
// produces output that looks identical to running
// `chronicle export <id>` once per session.
type BulkExportOptions struct {
	Provider          string
	HideTools         bool
	HideThinking      bool
	HideMeta          bool
	HideAwaySummaries bool
	HideFileContext   bool
}

// BulkExportEntry is one rendered session in the bulk
// stream. The CLI callback writes Content to disk under a
// filename it derives from SessionID and StartedAt, so
// composition stays free of any file-naming policy beyond
// passing the timestamp through.
type BulkExportEntry struct {
	SessionID contracts.SessionID
	Title     string
	StartedAt time.Time
	Content   string
}

// ErrProjectAmbiguous is returned by BulkExport when the
// requested project identifier exists under more than one
// provider and the caller did not pass a Provider hint to
// disambiguate. The CLI surfaces this as "use --provider"
// guidance so the user knows what to do next.
var ErrProjectAmbiguous = errors.New("project id matches more than one provider")

// BulkExport iterates every session in one project,
// renders each one as filtered Markdown, and yields the
// result to each. Iteration stops as soon as each returns
// a non-nil error. Composition never touches the
// filesystem: the callback is the only place a file gets
// written, which keeps the destination policy in the CLI
// where it belongs.
//
// Project lookup runs across every registered provider.
// When opts.Provider is empty and the project identifier
// matches under more than one provider, BulkExport returns
// ErrProjectAmbiguous so the CLI can ask the user to
// disambiguate. When the identifier matches nowhere,
// BulkExport returns an error wrapping fs.ErrNotExist.
//
// Sessions are processed in the order ListSessions returns
// them. For Claude that order is filename-sorted by
// session UUID, which is stable across runs but not
// chronological. The callback receives StartedAt so it can
// rename or sort the output if it wants chronological
// order on disk.
//
// Skipping unreadable sessions is left to the caller. Any
// ReadSession error aborts the whole bulk export, because
// silently dropping sessions in a backup-style operation
// would be worse than the loud failure.
func (a *App) BulkExport(projectID contracts.ProjectID, opts BulkExportOptions, each func(BulkExportEntry) error) (int, error) {
	if each == nil {
		return 0, errors.New("export bulk: callback is required")
	}

	// Step 1: resolve which provider owns the project. The
	// helper handles the disambiguation when more than one
	// provider knows the same id and surfaces a sentinel
	// error the CLI can translate into a clear message.
	provider, err := a.findProjectProvider(projectID, opts.Provider)
	if err != nil {
		return 0, err
	}

	// Step 2: enumerate every session under the project. We need
	// only the identifiers here — the loop below reads each session
	// in full — so this is the cheap ref listing, not a parse.
	refs, err := provider.Provider.ListSessionRefs(provider.FS, projectID)
	if err != nil {
		return 0, fmt.Errorf("export bulk: list sessions: %w", err)
	}

	// Step 3: stream each session through the read,
	// filter, and render pipeline, then hand the result to
	// the callback. The loop returns at the first callback
	// error so a failing destination aborts the bulk
	// operation instead of silently dropping later sessions.
	filterOpts := steps.FilterOptions{
		HideTools:         opts.HideTools,
		HideThinking:      opts.HideThinking,
		HideMeta:          opts.HideMeta,
		HideAwaySummaries: opts.HideAwaySummaries,
		HideFileContext:   opts.HideFileContext,
	}

	var written int
	for _, ref := range refs {
		conv, err := provider.Provider.ReadSession(provider.FS, ref.ID)
		if err != nil {
			return written, fmt.Errorf("export bulk: read %s: %w", ref.ID, err)
		}
		// The title and start time come from the full conversation,
		// matching the single-session export. The content renders
		// from the filtered copy.
		entry := BulkExportEntry{
			SessionID: ref.ID,
			Title:     conv.ListingTitle(),
			StartedAt: conv.StartedAt,
			Content:   steps.Markdown(steps.Filter(conv, filterOpts)),
		}
		if err := each(entry); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// findProjectProvider resolves which provider owns the
// requested project identifier. When the caller named a
// provider, only that one is checked. When the caller did
// not, every provider is consulted and the result depends
// on how many of them know the identifier:
//
//   - exactly one match: that provider is returned
//   - zero matches: an error wrapping fs.ErrNotExist
//   - more than one match: ErrProjectAmbiguous
//
// The two error sentinels let the CLI render distinct
// messages without parsing strings out of the wrap.
func (a *App) findProjectProvider(projectID contracts.ProjectID, providerName string) (*providerEntry, error) {
	var matches []*providerEntry
	for _, p := range a.providers {
		if providerName != "" && p.Provider.Name() != providerName {
			continue
		}
		projects, err := p.Provider.ListProjects(p.FS)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("export bulk: list projects in %s: %w", p.Provider.Name(), err)
		}
		for _, proj := range projects {
			if proj.ID == projectID {
				matches = append(matches, p)
				break
			}
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("export bulk: project %q: %w", projectID, fs.ErrNotExist)
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("export bulk: project %q: %w", projectID, ErrProjectAmbiguous)
	}
}
