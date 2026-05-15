package composition

import (
	"io/fs"
	"testing"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/internal/config"
	"github.com/danieljbfz/chronicle/internal/paths"
)

// statsFake is the smallest Provider that satisfies what
// Stats needs. ReadSession is never called by the stats
// path, so we leave it returning fs.ErrNotExist as a tripwire
// in case a future regression starts reading sessions.
type statsFake struct {
	name     string
	projects []contracts.Project
	sessions map[contracts.ProjectID][]contracts.SessionSummary
}

func (f *statsFake) Name() string { return f.name }
func (f *statsFake) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{}, nil
}
func (f *statsFake) ListProjects(fs.FS) ([]contracts.Project, error) {
	return f.projects, nil
}
func (f *statsFake) ListSessions(_ fs.FS, p contracts.ProjectID) ([]contracts.SessionSummary, error) {
	return f.sessions[p], nil
}
func (f *statsFake) ReadSession(fs.FS, contracts.SessionID) (contracts.Conversation, error) {
	return contracts.Conversation{}, fs.ErrNotExist
}

// makeStatsApp wires statsFakes into an App. The pattern
// matches makeSearchApp so the two test files read the same
// way and a future maintainer can move from one to the
// other without learning a new convention.
func makeStatsApp(t *testing.T, fakes ...*statsFake) *App {
	t.Helper()
	a := &App{
		settings:  config.Defaults(),
		locations: paths.Locations{TrashDir: t.TempDir()},
	}
	for _, f := range fakes {
		a.providers = append(a.providers, &providerEntry{
			Provider: f,
			Root:     t.TempDir(),
		})
	}
	return a
}

// at is a one-line helper for deterministic timestamps in the
// tests. The dates here are arbitrary, but chosen far apart
// so the oldest/newest assertions cannot pass by accident.
func at(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 12, 0, 0, 0, time.UTC)
}

// TestStats_aggregatesAcrossProviders covers the happy path.
// Two providers, two projects each, with distinct session
// counts and sizes. The total row should be the sum, the
// per-provider rows should match what each fake returned, and
// the date range should span the full set of sessions.
func TestStats_aggregatesAcrossProviders(t *testing.T) {
	claudeFake := &statsFake{
		name: "claude",
		projects: []contracts.Project{
			{ID: "alpha", DisplayName: "alpha"},
			{ID: "bravo", DisplayName: "bravo"},
		},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"alpha": {
				{ID: "a1", Project: "alpha", TurnCount: 10, SizeBytes: 1000, StartedAt: at(2025, 1, 1), LastActive: at(2025, 1, 2)},
				{ID: "a2", Project: "alpha", TurnCount: 20, SizeBytes: 2000, StartedAt: at(2025, 2, 1), LastActive: at(2025, 2, 3)},
			},
			"bravo": {
				{ID: "b1", Project: "bravo", TurnCount: 5, SizeBytes: 500, StartedAt: at(2025, 3, 1), LastActive: at(2025, 3, 1)},
			},
		},
	}
	copilotFake := &statsFake{
		name: "copilot",
		projects: []contracts.Project{
			{ID: "charlie", DisplayName: "charlie"},
		},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"charlie": {
				{ID: "c1", Project: "charlie", TurnCount: 7, SizeBytes: 700, StartedAt: at(2024, 12, 25), LastActive: at(2025, 4, 1)},
			},
		},
	}
	a := makeStatsApp(t, claudeFake, copilotFake)

	stats, err := a.Stats(StatsOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if got, want := stats.Total.Sessions, 4; got != want {
		t.Errorf("total sessions = %d, want %d", got, want)
	}
	if got, want := stats.Total.Messages, 42; got != want {
		t.Errorf("total messages = %d, want %d", got, want)
	}
	if got, want := stats.Total.SizeBytes, int64(4200); got != want {
		t.Errorf("total bytes = %d, want %d", got, want)
	}
	if !stats.Total.OldestAt.Equal(at(2024, 12, 25)) {
		t.Errorf("oldest = %v, want 2024-12-25", stats.Total.OldestAt)
	}
	if !stats.Total.NewestAt.Equal(at(2025, 4, 1)) {
		t.Errorf("newest = %v, want 2025-04-01", stats.Total.NewestAt)
	}
	if len(stats.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(stats.Providers))
	}
	if stats.Providers[0].Name != "claude" || stats.Providers[1].Name != "copilot" {
		t.Errorf("provider order = [%s, %s], want [claude, copilot]", stats.Providers[0].Name, stats.Providers[1].Name)
	}
	if got, want := stats.Providers[0].Aggregate.Sessions, 3; got != want {
		t.Errorf("claude sessions = %d, want %d", got, want)
	}
	if got, want := stats.Providers[1].Aggregate.Sessions, 1; got != want {
		t.Errorf("copilot sessions = %d, want %d", got, want)
	}
}

// TestStats_filtersByProvider confirms the --provider scoping.
// Both providers have sessions, but the result only carries
// the named one and the totals match that one provider.
func TestStats_filtersByProvider(t *testing.T) {
	claudeFake := &statsFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "alpha"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"alpha": {{ID: "a1", Project: "alpha", TurnCount: 10, SizeBytes: 1000}},
		},
	}
	copilotFake := &statsFake{
		name:     "copilot",
		projects: []contracts.Project{{ID: "charlie"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"charlie": {{ID: "c1", Project: "charlie", TurnCount: 7, SizeBytes: 700}},
		},
	}
	a := makeStatsApp(t, claudeFake, copilotFake)

	stats, err := a.Stats(StatsOptions{Provider: "copilot"})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Providers) != 1 || stats.Providers[0].Name != "copilot" {
		t.Fatalf("providers = %v, want [copilot]", stats.Providers)
	}
	if stats.Total.Sessions != 1 || stats.Total.Messages != 7 {
		t.Errorf("totals = (sessions=%d, messages=%d), want (1, 7)", stats.Total.Sessions, stats.Total.Messages)
	}
}

// TestStats_topProjectsRespectN proves the TopN cap. Three
// projects ranked by session count, and the request asks for
// the top two. The most-active project comes first, the
// second-most-active second, and the third never appears.
func TestStats_topProjectsRespectN(t *testing.T) {
	fake := &statsFake{
		name: "claude",
		projects: []contracts.Project{
			{ID: "small", DisplayName: "small"},
			{ID: "big", DisplayName: "big"},
			{ID: "medium", DisplayName: "medium"},
		},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"small":  {{ID: "s1", Project: "small"}},
			"big":    {{ID: "b1", Project: "big"}, {ID: "b2", Project: "big"}, {ID: "b3", Project: "big"}},
			"medium": {{ID: "m1", Project: "medium"}, {ID: "m2", Project: "medium"}},
		},
	}
	a := makeStatsApp(t, fake)

	stats, err := a.Stats(StatsOptions{TopN: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.TopProjects) != 2 {
		t.Fatalf("top = %d, want 2", len(stats.TopProjects))
	}
	if stats.TopProjects[0].DisplayName != "big" {
		t.Errorf("top[0] = %q, want big", stats.TopProjects[0].DisplayName)
	}
	if stats.TopProjects[1].DisplayName != "medium" {
		t.Errorf("top[1] = %q, want medium", stats.TopProjects[1].DisplayName)
	}
}

// TestStats_topProjectsBreakTiesByName pins the secondary
// sort so the output is reproducible across runs. Two
// projects with identical session counts come back in
// alphabetical order by display name.
func TestStats_topProjectsBreakTiesByName(t *testing.T) {
	fake := &statsFake{
		name: "claude",
		projects: []contracts.Project{
			{ID: "zeta", DisplayName: "zeta"},
			{ID: "alpha", DisplayName: "alpha"},
		},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"zeta":  {{ID: "z1"}, {ID: "z2"}},
			"alpha": {{ID: "a1"}, {ID: "a2"}},
		},
	}
	a := makeStatsApp(t, fake)

	stats, err := a.Stats(StatsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.TopProjects[0].DisplayName != "alpha" {
		t.Errorf("first = %q, want alpha (alphabetical tiebreak)", stats.TopProjects[0].DisplayName)
	}
}

// TestStats_emptyProvidersReturnZeroes confirms a clean
// summary against zero data. The result should still be a
// well-formed Stats with empty totals and a non-zero
// GeneratedAt timestamp so JSON consumers do not have to
// special-case "no data".
func TestStats_emptyProvidersReturnZeroes(t *testing.T) {
	fake := &statsFake{name: "claude"}
	a := makeStatsApp(t, fake)

	stats, err := a.Stats(StatsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total.Sessions != 0 || stats.Total.Messages != 0 || stats.Total.SizeBytes != 0 {
		t.Errorf("totals = %+v, want zeros", stats.Total)
	}
	if !stats.Total.OldestAt.IsZero() || !stats.Total.NewestAt.IsZero() {
		t.Errorf("date range = (%v, %v), want zero values", stats.Total.OldestAt, stats.Total.NewestAt)
	}
	if stats.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should always be set")
	}
	if len(stats.Providers) != 1 || stats.Providers[0].Name != "claude" {
		t.Errorf("providers = %v, want one entry for claude", stats.Providers)
	}
}

// TestStats_emptyProjectStillCountsButNotInTop confirms a
// subtle rule: an empty project bumps the per-provider
// project count but does not appear as a top-projects row.
// Otherwise the table would be polluted with zero-session
// entries that tell the user nothing.
func TestStats_emptyProjectStillCountsButNotInTop(t *testing.T) {
	fake := &statsFake{
		name: "claude",
		projects: []contracts.Project{
			{ID: "empty", DisplayName: "empty"},
			{ID: "active", DisplayName: "active"},
		},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"empty":  {},
			"active": {{ID: "x1"}},
		},
	}
	a := makeStatsApp(t, fake)

	stats, err := a.Stats(StatsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := stats.Providers[0].Projects, 2; got != want {
		t.Errorf("project count = %d, want %d", got, want)
	}
	if len(stats.TopProjects) != 1 {
		t.Fatalf("top projects = %d, want 1 (empty project excluded)", len(stats.TopProjects))
	}
	if stats.TopProjects[0].DisplayName != "active" {
		t.Errorf("top[0] = %q, want active", stats.TopProjects[0].DisplayName)
	}
}

// TestStats_dateRangeIgnoresZeroValues confirms a single
// missing timestamp does not poison the range. One session
// has both timestamps set, one has neither. The range
// should reflect only the session whose data is present.
func TestStats_dateRangeIgnoresZeroValues(t *testing.T) {
	fake := &statsFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "p"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p": {
				{ID: "s1", StartedAt: at(2025, 6, 1), LastActive: at(2025, 6, 2)},
				{ID: "s2"},
			},
		},
	}
	a := makeStatsApp(t, fake)

	stats, err := a.Stats(StatsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Total.OldestAt.Equal(at(2025, 6, 1)) {
		t.Errorf("oldest = %v, want 2025-06-01", stats.Total.OldestAt)
	}
	if !stats.Total.NewestAt.Equal(at(2025, 6, 2)) {
		t.Errorf("newest = %v, want 2025-06-02", stats.Total.NewestAt)
	}
}
