package composition

import (
	"fmt"
	"sort"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/steps"
)

// SearchOptions controls Search. Every field has a sensible
// zero-value default so the CLI can pass a bare struct for
// the common case.
type SearchOptions struct {
	// Provider, when non-empty, restricts the search to one
	// adapter by its registered name ("claude",
	// "copilot-chat", or "copilot-agent"). The CLI exposes
	// this through `--provider`. Most users have a few
	// hundred sessions across the registered tools and
	// want every match by default, so the empty value is
	// the right shape for the common case.
	Provider string

	// MaxSnippetsPerSession caps how many matches the
	// renderer should print per session. We pass it through
	// to steps.Match so the limit applies during the
	// scan, not after. The default of zero falls back to a
	// small per-session cap so the listing stays readable
	// when one session matches the query dozens of times.
	MaxSnippetsPerSession int

	// CaseSensitive forwards to steps.Match. The default is
	// false (case folded) because most users typing
	// `chronicle search refactor` will not remember whether
	// the original text said "refactor" or "Refactor".
	CaseSensitive bool
}

// defaultMaxSnippetsPerSession is the cap the CLI applies
// when the caller does not pass an explicit value. Three
// matches is enough for the user to recognize the session
// without flooding the terminal.
const defaultMaxSnippetsPerSession = 3

// SearchResult is one matching session, paired with the
// snippets that matched. The CLI renders one result per
// line for the session header and one indented line per
// snippet, so this is the shape that lines up with the
// rendering layer.
type SearchResult struct {
	Provider  string
	SessionID contracts.SessionID
	ProjectID contracts.ProjectID
	Title     string
	Snippets  []steps.SearchSnippet
}

// Search walks every session across every detected provider
// (or one provider, if SearchOptions.Provider is set) and
// returns the sessions whose text content contains the
// query. The result is sorted by provider then by session
// title so the output reads in a stable order across runs.
//
// Empty queries return an error rather than every session,
// because an empty match would be useless and would hide
// any typo at the CLI from the user. The error is plain so
// the CLI can present it as "search: empty query" without
// fishing for a typed error.
//
// Search is read-only. It calls ListProjects, ListSessions,
// and ReadSession on each provider and applies the pure
// step in steps/search.go to filter the content.
func (a *App) Search(query string, opts SearchOptions) ([]SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("search: query must not be empty")
	}
	maxSnippets := opts.MaxSnippetsPerSession
	if maxSnippets == 0 {
		maxSnippets = defaultMaxSnippetsPerSession
	}
	matchOpts := steps.SearchOptions{
		CaseSensitive:         opts.CaseSensitive,
		MaxSnippetsPerSession: maxSnippets,
	}

	var results []SearchResult
	for _, p := range a.providers {
		if opts.Provider != "" && p.Provider.Name() != opts.Provider {
			continue
		}
		more, err := searchProvider(p, query, matchOpts)
		if err != nil {
			return nil, fmt.Errorf("search %s: %w", p.Provider.Name(), err)
		}
		results = append(results, more...)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Provider != results[j].Provider {
			return results[i].Provider < results[j].Provider
		}
		return results[i].Title < results[j].Title
	})
	return results, nil
}

// searchProvider walks every session of one provider and
// returns the ones whose content matches the query. We
// split this out from Search so the per-provider loop
// stays short and the parent function reads top-to-bottom
// without nested loops.
//
// The function reads every session from the provider one at
// a time. On the contributor's machine that is around 180
// sessions for two providers. The serial walk is fast
// enough that we have not needed parallelism here. If a
// future install grows to thousands of sessions, the right
// fix will be either an errgroup fan-out or a small index
// that the search consults instead of every file.
func searchProvider(p *providerEntry, query string, matchOpts steps.SearchOptions) ([]SearchResult, error) {
	projects, err := p.Provider.ListProjects(p.FS)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, project := range projects {
		summaries, err := p.Provider.ListSessions(p.FS, project.ID)
		if err != nil {
			return nil, err
		}
		for _, summary := range summaries {
			conv, err := p.Provider.ReadSession(p.FS, summary.ID)
			if err != nil {
				// One bad session should not bury the rest.
				// We keep going and rely on the doctor view
				// for surfacing per-session read failures.
				continue
			}
			snippets := steps.Match(conv, query, matchOpts)
			if len(snippets) == 0 {
				continue
			}
			results = append(results, SearchResult{
				Provider:  p.Provider.Name(),
				SessionID: summary.ID,
				ProjectID: summary.Project,
				Title:     summary.Title,
				Snippets:  snippets,
			})
		}
	}
	return results, nil
}
