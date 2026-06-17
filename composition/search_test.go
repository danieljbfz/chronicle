package composition

import (
	"io/fs"
	"testing"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/internal/config"
	"github.com/danieljbfz/chronicle/internal/paths"
)

// searchFake is a Provider tuned for the search tests. The
// fake stores conversations keyed by session ID and serves
// them through ReadSession, while ListProjects and
// ListSessions return whatever the test put in.
type searchFake struct {
	name     string
	projects []contracts.Project
	sessions map[contracts.ProjectID][]contracts.SessionSummary
	convos   map[contracts.SessionID]contracts.Conversation
}

func (f *searchFake) Name() string { return f.name }
func (f *searchFake) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{}, nil
}
func (f *searchFake) ListProjects(fs.FS) ([]contracts.Project, error) {
	return f.projects, nil
}
func (f *searchFake) ListSessionRefs(_ fs.FS, p contracts.ProjectID) ([]contracts.SessionRef, error) {
	var refs []contracts.SessionRef
	for _, s := range f.sessions[p] {
		refs = append(refs, contracts.SessionRef{ID: s.ID, Project: p, SizeBytes: s.SizeBytes})
	}
	return refs, nil
}
func (f *searchFake) SummarizeSession(_ fs.FS, ref contracts.SessionRef) (contracts.SessionSummary, error) {
	for _, s := range f.sessions[ref.Project] {
		if s.ID == ref.ID {
			return s, nil
		}
	}
	return contracts.SessionSummary{}, fs.ErrNotExist
}
func (f *searchFake) ReadSession(_ fs.FS, id contracts.SessionID) (contracts.Conversation, error) {
	c, ok := f.convos[id]
	if !ok {
		return contracts.Conversation{}, fs.ErrNotExist
	}
	return c, nil
}

// makeSearchApp wraps one or more searchFakes into an App
// suitable for the tests below. We pass real
// TrashDir-bearing paths even though search never touches
// the trash, because building the App with the production
// types keeps the test path close to production.
func makeSearchApp(t *testing.T, fakes ...*searchFake) *App {
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

// convWithText is a tiny helper to construct a Conversation
// from one user prompt and one assistant reply. Most search
// tests only need a single round-trip, so the helper saves
// repetition.
func convWithText(prompt, reply string) contracts.Conversation {
	return contracts.Conversation{
		Messages: []contracts.Message{
			{Role: contracts.RoleUser, Blocks: []contracts.Block{contracts.TextBlock{Text: prompt}}},
			{Role: contracts.RoleAssistant, Blocks: []contracts.Block{contracts.TextBlock{Text: reply}}},
		},
	}
}

// TestSearch_findsMatchingSessionAcrossProvider is the happy
// path. One provider, two sessions, only one of which
// contains the query. The result has one entry, and that
// entry carries snippets pointing at the match.
func TestSearch_findsMatchingSessionAcrossProvider(t *testing.T) {
	fake := &searchFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "p1"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p1": {
				{ID: "s1", Project: "p1", Title: "About Go file I/O"},
				{ID: "s2", Project: "p1", Title: "About Python decorators"},
			},
		},
		convos: map[contracts.SessionID]contracts.Conversation{
			"s1": convWithText("How do I read a file in Go?", "Use os.ReadFile."),
			"s2": convWithText("What's a decorator?", "It wraps a function."),
		},
	}
	a := makeSearchApp(t, fake)

	results, err := a.Search("Go", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1 (only s1 mentions Go)", len(results))
	}
	if results[0].SessionID != "s1" {
		t.Errorf("session id = %q, want s1", results[0].SessionID)
	}
	if len(results[0].Snippets) == 0 {
		t.Error("expected at least one snippet")
	}
}

// TestSearch_emptyQueryReturnsError pins the contract. An
// empty query at the CLI is almost always a typo, and we
// would rather surface it than dump every session as a
// match.
func TestSearch_emptyQueryReturnsError(t *testing.T) {
	a := makeSearchApp(t, &searchFake{name: "claude"})
	_, err := a.Search("", SearchOptions{})
	if err == nil {
		t.Error("expected an error for an empty query")
	}
}

// TestSearch_filtersByProviderName confirms the --provider
// flag scopes the result. Two providers, both with a
// matching session, but the search only returns the
// matching session from the named provider.
func TestSearch_filtersByProviderName(t *testing.T) {
	claudeFake := &searchFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "p1"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p1": {{ID: "s1", Project: "p1"}},
		},
		convos: map[contracts.SessionID]contracts.Conversation{
			"s1": convWithText("chronicle question", "chronicle answer"),
		},
	}
	copilotFake := &searchFake{
		name:     "copilot",
		projects: []contracts.Project{{ID: "p2"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p2": {{ID: "s2", Project: "p2"}},
		},
		convos: map[contracts.SessionID]contracts.Conversation{
			"s2": convWithText("chronicle prompt", "chronicle reply"),
		},
	}
	a := makeSearchApp(t, claudeFake, copilotFake)

	results, err := a.Search("chronicle", SearchOptions{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1 (only claude)", len(results))
	}
	if results[0].Provider != "claude" {
		t.Errorf("provider = %q, want claude", results[0].Provider)
	}
}

// TestSearch_resultsAreStableAcrossRuns confirms the sort
// order. Two matching sessions in the same provider come
// back in alphabetical order by title, so consecutive
// invocations of the CLI produce identical output.
func TestSearch_resultsAreStableAcrossRuns(t *testing.T) {
	fake := &searchFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "p1"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p1": {
				{ID: "s1", Project: "p1"},
				{ID: "s2", Project: "p1"},
			},
		},
		// The title a result carries is the conversation's listing
		// title, the same value the listing surfaces show. Each
		// session's first prompt both carries the search term and
		// determines the title, so the two sort alphabetically.
		convos: map[contracts.SessionID]contracts.Conversation{
			"s1": convWithText("Beta session about chronicle", "yes"),
			"s2": convWithText("Alpha session about chronicle", "yes"),
		},
	}
	a := makeSearchApp(t, fake)
	results, err := a.Search("chronicle", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].Title != "Alpha session about chronicle" {
		t.Errorf("first result title = %q, want the alphabetically-first title", results[0].Title)
	}
}

// TestSearch_skipsUnreadableSessionsAndContinues confirms
// the resilience contract. A session that fails to read
// must not abort the whole search. The failed session
// gets skipped while the other sessions come through.
func TestSearch_skipsUnreadableSessionsAndContinues(t *testing.T) {
	fake := &searchFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "p1"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p1": {
				{ID: "broken", Project: "p1"},
				{ID: "good", Project: "p1"},
			},
		},
		convos: map[contracts.SessionID]contracts.Conversation{
			"good": convWithText("chronicle", "yes"),
			// "broken" is missing from convos, so ReadSession
			// returns fs.ErrNotExist.
		},
	}
	a := makeSearchApp(t, fake)
	results, err := a.Search("chronicle", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("results = %d, want 1 (broken session skipped)", len(results))
	}
}
