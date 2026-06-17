package composition

import (
	"io/fs"
	"strconv"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/internal/paths"
)

// countingProvider records how many times SummarizeSession runs, so a
// test can prove a second listing served the cache instead of re-parsing.
// It exposes one session whose ref is stable across calls.
type countingProvider struct {
	summary contracts.SessionSummary
	mu      sync.Mutex
	calls   int
}

func (*countingProvider) Name() string { return "counting" }
func (*countingProvider) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{Adapter: "counting", Version: "1", Fingerprint: "fp"}, nil
}
func (*countingProvider) ListProjects(fs.FS) ([]contracts.Project, error) {
	return []contracts.Project{{ID: "proj"}}, nil
}
func (*countingProvider) ListSessionRefs(fs.FS, contracts.ProjectID) ([]contracts.SessionRef, error) {
	return []contracts.SessionRef{{ID: "s1", Project: "proj", SizeBytes: 10, ModTime: time.Unix(1000, 0)}}, nil
}
func (c *countingProvider) SummarizeSession(fs.FS, contracts.SessionRef) (contracts.SessionSummary, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return c.summary, nil
}
func (*countingProvider) ReadSession(fs.FS, contracts.SessionID) (contracts.Conversation, error) {
	return contracts.Conversation{}, nil
}

func (c *countingProvider) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// TestSummariesFor_secondListingServesTheCache proves the cache earns its
// keep: across two listings of an unchanged session, the expensive
// SummarizeSession runs exactly once.
func TestSummariesFor_secondListingServesTheCache(t *testing.T) {
	cp := &countingProvider{summary: contracts.SessionSummary{ID: "s1", Project: "proj", Title: "kept", LastActive: time.Unix(1000, 0)}}
	app := NewForTest([]contracts.Provider{cp}, []fs.FS{fstest.MapFS{}})
	app.locations = paths.Locations{CacheDir: t.TempDir()}
	app.providers[0].Version = contracts.StorageVersion{Fingerprint: "fp"}

	if _, err := app.ListSessionsAll(""); err != nil {
		t.Fatalf("first listing: %v", err)
	}
	if _, err := app.ListSessionsAll(""); err != nil {
		t.Fatalf("second listing: %v", err)
	}

	if got := cp.callCount(); got != 1 {
		t.Fatalf("SummarizeSession ran %d times, want 1 (the second listing should hit the cache)", got)
	}
}

// manyProvider exposes a configurable number of sessions, each a cache
// miss when no cache directory is set, so the parallel parse path runs
// under the race detector.
type manyProvider struct{ count int }

func (*manyProvider) Name() string { return "many" }
func (*manyProvider) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{Fingerprint: "fp"}, nil
}
func (*manyProvider) ListProjects(fs.FS) ([]contracts.Project, error) {
	return []contracts.Project{{ID: "proj"}}, nil
}
func (m *manyProvider) ListSessionRefs(fs.FS, contracts.ProjectID) ([]contracts.SessionRef, error) {
	refs := make([]contracts.SessionRef, m.count)
	for i := range refs {
		refs[i] = contracts.SessionRef{ID: contracts.SessionID(strconv.Itoa(i)), Project: "proj", SizeBytes: int64(i)}
	}
	return refs, nil
}
func (*manyProvider) SummarizeSession(_ fs.FS, ref contracts.SessionRef) (contracts.SessionSummary, error) {
	return contracts.SessionSummary{ID: ref.ID, Project: ref.Project}, nil
}
func (*manyProvider) ReadSession(fs.FS, contracts.SessionID) (contracts.Conversation, error) {
	return contracts.Conversation{}, nil
}

// TestSummariesFor_parsesManyMissesUnderRace runs every session as a
// cache miss so the parallel parse path is exercised. Run under
// `go test -race`, it guards the per-slot write discipline against a
// data race, and it confirms no session is dropped under concurrency.
func TestSummariesFor_parsesManyMissesUnderRace(t *testing.T) {
	app := NewForTest([]contracts.Provider{&manyProvider{count: 50}}, []fs.FS{fstest.MapFS{}})
	app.providers[0].Version = contracts.StorageVersion{Fingerprint: "fp"}

	listings, err := app.ListSessionsAll("")
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(listings) != 50 {
		t.Fatalf("got %d summaries, want 50", len(listings))
	}
}

// TestApp_concurrentListingAndStats_isRaceFree pins the property the TUI
// depends on: it loads its sessions and stats screens through separate
// commands, so ListSessionsAll and Stats can run against the same App at
// once. Both share the lazily loaded summary cache. Run under
// `go test -race`, this catches a race on the cache handle, its entries,
// or the flush. A real cache directory is set so the persistence path
// (load-once, get, put, flush) is exercised, not just the in-memory one.
func TestApp_concurrentListingAndStats_isRaceFree(t *testing.T) {
	app := NewForTest([]contracts.Provider{&manyProvider{count: 30}}, []fs.FS{fstest.MapFS{}})
	app.locations = paths.Locations{CacheDir: t.TempDir()}
	app.providers[0].Version = contracts.StorageVersion{Fingerprint: "fp"}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := app.ListSessionsAll(""); err != nil {
				t.Errorf("listing: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := app.Stats(StatsOptions{}); err != nil {
				t.Errorf("stats: %v", err)
			}
		}()
	}
	wg.Wait()
}

// TestSummariesFor_sortsNewestFirst pins the listing order that the
// session browser depends on: the most recently active session leads.
// The sort lives in summariesForProject now rather than in each adapter, so
// the coverage lives here too. The fake returns its sessions out of
// order on purpose, so a passing test means the sort, not the fake,
// produced the order.
func TestSummariesFor_sortsNewestFirst(t *testing.T) {
	at := func(month time.Month) time.Time {
		return time.Date(2026, month, 1, 0, 0, 0, 0, time.UTC)
	}
	fake := &fakeProvider{
		name:     "claude",
		projects: []contracts.Project{{ID: "p1"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p1": {
				{ID: "old", Project: "p1", LastActive: at(time.January)},
				{ID: "recent", Project: "p1", LastActive: at(time.June)},
				{ID: "mid", Project: "p1", LastActive: at(time.March)},
			},
		},
	}

	listings, err := newAppWithFakes(fake).ListSessionsAll("")
	if err != nil {
		t.Fatal(err)
	}

	want := []contracts.SessionID{"recent", "mid", "old"}
	if len(listings) != len(want) {
		t.Fatalf("got %d listings, want %d", len(listings), len(want))
	}
	for i, id := range want {
		if listings[i].Summary.ID != id {
			t.Errorf("position %d = %q, want %q", i, listings[i].Summary.ID, id)
		}
	}
}
