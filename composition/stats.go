package composition

import (
	"errors"
	"io/fs"
	"sort"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

// StatsOptions controls the Stats call. The zero value is a
// valid request: no provider filter and the default top-N
// projects. The CLI exposes both fields as flags.
type StatsOptions struct {
	// Provider, when non-empty, restricts the summary to one
	// adapter by its registered name (such as "claude",
	// "copilot-chat", or "copilot-agent"). Most users have
	// a few providers installed and want the full picture
	// across all of them, so the empty string is the right
	// default.
	Provider string

	// TopN sets how many projects appear in the top-projects
	// section. Zero falls back to defaultStatsTopN. The CLI
	// uses this to bound the table so a user with hundreds
	// of projects still gets a one-screen summary.
	TopN int
}

// defaultStatsTopN is the number of projects in the top-N
// table when the caller does not pick a value. Five is a
// readable size that fits below the totals on a normal
// terminal without scrolling.
const defaultStatsTopN = 5

// Aggregate is the running total chronicle accumulates as it
// walks sessions. The same struct is reused at three scopes:
// per-project, per-provider, and total. Reusing one type
// keeps the rendering code uniform, and the rendering code is
// where most of the Stats output is shaped.
type Aggregate struct {
	Sessions  int
	Messages  int
	SizeBytes int64

	// OldestAt and NewestAt are the earliest StartedAt and
	// the latest LastActive values seen across the
	// underlying sessions. Zero-valued timestamps from a
	// summary are ignored so a single missing timestamp
	// does not skew the range.
	OldestAt time.Time
	NewestAt time.Time
}

// add folds one session summary into the aggregate. We keep
// this method on Aggregate so the per-project, per-provider,
// and total accumulations all use the same logic. If we ever
// change how a missing timestamp is handled, there is one
// place to edit.
func (a *Aggregate) add(s contracts.SessionSummary) {
	a.Sessions++
	a.Messages += s.TurnCount
	a.SizeBytes += s.SizeBytes
	if !s.StartedAt.IsZero() && (a.OldestAt.IsZero() || s.StartedAt.Before(a.OldestAt)) {
		a.OldestAt = s.StartedAt
	}
	if !s.LastActive.IsZero() && s.LastActive.After(a.NewestAt) {
		a.NewestAt = s.LastActive
	}
}

// ProviderStats is the per-adapter slice of the summary. The
// Aggregate field carries every metric in the same shape as
// the totals row, so the renderer can treat the per-provider
// rows and the total row with one code path. Projects is the
// number of distinct projects this provider exposes.
type ProviderStats struct {
	Name      string
	Projects  int
	Aggregate Aggregate
}

// ProjectStats is one row of the top-projects table. We carry
// the human-readable DisplayName and the on-disk Path
// alongside the identifier so the CLI can pick whichever the
// user finds more useful at render time.
type ProjectStats struct {
	Provider    string
	ProjectID   contracts.ProjectID
	DisplayName string
	Path        string
	Aggregate   Aggregate
}

// Stats is the full result of one App.Stats call. The shape
// is meant to be rendered directly by the CLI and to round-
// trip through JSON without losing fidelity, so every field
// on Stats and on the embedded Aggregates carries its own
// JSON tag.
type Stats struct {
	GeneratedAt time.Time
	Total       Aggregate
	Providers   []ProviderStats
	TopProjects []ProjectStats
}

// Stats walks every detected provider and returns a one-shot
// summary built from session summaries alone. It does not
// call ReadSession, which means the cost is bounded by the
// listing cost rather than by the JSON-parse cost of every
// session on disk. On the contributor's machine this is the
// difference between sub-second and several-second response
// times for a few hundred sessions.
//
// The returned Providers slice is in registration order so
// the CLI output is stable across runs. TopProjects is
// sorted by session count descending, then by display name
// ascending so ties resolve deterministically.
//
// A provider whose data directory has gone missing since
// Detect is skipped, the same way ListSessionsAll handles
// it. Any other listing error is returned to the caller.
func (a *App) Stats(opts StatsOptions) (Stats, error) {
	topN := opts.TopN
	if topN == 0 {
		topN = defaultStatsTopN
	}

	out := Stats{GeneratedAt: time.Now().UTC()}
	var allProjects []ProjectStats

	for _, p := range a.providers {
		if opts.Provider != "" && p.Provider.Name() != opts.Provider {
			continue
		}
		ps, projects, err := statsForProvider(p)
		if err != nil {
			return Stats{}, err
		}
		out.Providers = append(out.Providers, ps)
		out.Total.Sessions += ps.Aggregate.Sessions
		out.Total.Messages += ps.Aggregate.Messages
		out.Total.SizeBytes += ps.Aggregate.SizeBytes
		mergeRange(&out.Total, ps.Aggregate)
		allProjects = append(allProjects, projects...)
	}

	out.TopProjects = topProjects(allProjects, topN)
	return out, nil
}

// statsForProvider walks one provider's projects and sessions
// and returns the rolled-up provider stats together with the
// per-project rows. We split this out so the parent loop
// stays linear and so testing one provider does not have to
// build an App with multiple registered providers.
//
// The function returns an empty ProviderStats and a nil
// project slice when the provider's root directory does not
// exist, mirroring the same fs.ErrNotExist tolerance the
// other composition methods apply.
func statsForProvider(p *providerEntry) (ProviderStats, []ProjectStats, error) {
	ps := ProviderStats{Name: p.Provider.Name()}

	projects, err := p.Provider.ListProjects(p.FS)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ps, nil, nil
		}
		return ProviderStats{}, nil, err
	}

	rows := make([]ProjectStats, 0, len(projects))
	for _, project := range projects {
		summaries, err := p.Provider.ListSessions(p.FS, project.ID)
		if err != nil {
			return ProviderStats{}, nil, err
		}
		if len(summaries) == 0 {
			// An empty project still counts toward the
			// project count but contributes nothing to the
			// per-session aggregates. Skipping the row keeps
			// the top-projects table free of zero entries.
			ps.Projects++
			continue
		}

		row := ProjectStats{
			Provider:    p.Provider.Name(),
			ProjectID:   project.ID,
			DisplayName: project.DisplayName,
			Path:        project.Path,
		}
		for _, s := range summaries {
			row.Aggregate.add(s)
			ps.Aggregate.add(s)
		}
		ps.Projects++
		rows = append(rows, row)
	}

	return ps, rows, nil
}

// mergeRange folds the OldestAt/NewestAt of one aggregate
// into another. The numeric fields are summed by the caller
// because addition is associative across providers. The date
// range needs min/max instead, which is exactly what this
// helper handles.
func mergeRange(into *Aggregate, from Aggregate) {
	if !from.OldestAt.IsZero() && (into.OldestAt.IsZero() || from.OldestAt.Before(into.OldestAt)) {
		into.OldestAt = from.OldestAt
	}
	if !from.NewestAt.IsZero() && from.NewestAt.After(into.NewestAt) {
		into.NewestAt = from.NewestAt
	}
}

// topProjects sorts the cross-provider project list by
// session count descending, with display name as the
// tie-breaker, and returns the first n rows. We sort a copy
// instead of mutating the caller's slice because the caller
// may want the unsorted list later for a different view, and
// the cost of one allocation here is negligible next to the
// listing work that produced the slice.
func topProjects(projects []ProjectStats, n int) []ProjectStats {
	if len(projects) == 0 || n <= 0 {
		return nil
	}
	sorted := make([]ProjectStats, len(projects))
	copy(sorted, projects)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Aggregate.Sessions != sorted[j].Aggregate.Sessions {
			return sorted[i].Aggregate.Sessions > sorted[j].Aggregate.Sessions
		}
		return sorted[i].DisplayName < sorted[j].DisplayName
	})
	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}
