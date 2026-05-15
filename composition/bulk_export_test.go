package composition

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/internal/config"
	"github.com/danieljbfz/chronicle/internal/paths"
)

// bulkFake is the smallest Provider that supports BulkExport.
// projects is the list ListProjects returns. summaries maps
// project IDs to their session summaries. convos maps session
// IDs to a Conversation that ReadSession serves back. We
// build conversations with a single user/assistant exchange
// so the rendered Markdown has predictable substrings the
// tests can assert on.
type bulkFake struct {
	name      string
	projects  []contracts.Project
	summaries map[contracts.ProjectID][]contracts.SessionSummary
	convos    map[contracts.SessionID]contracts.Conversation
}

func (f *bulkFake) Name() string { return f.name }
func (f *bulkFake) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{}, nil
}
func (f *bulkFake) ListProjects(fs.FS) ([]contracts.Project, error) {
	return f.projects, nil
}
func (f *bulkFake) ListSessions(_ fs.FS, p contracts.ProjectID) ([]contracts.SessionSummary, error) {
	return f.summaries[p], nil
}
func (f *bulkFake) ReadSession(_ fs.FS, id contracts.SessionID) (contracts.Conversation, error) {
	c, ok := f.convos[id]
	if !ok {
		return contracts.Conversation{}, fs.ErrNotExist
	}
	return c, nil
}

// makeBulkApp wires bulkFakes into an App. The pattern
// mirrors the other composition test helpers so a new
// reader can move from one to the next without learning a
// new convention.
func makeBulkApp(t *testing.T, fakes ...*bulkFake) *App {
	t.Helper()
	a := &App{
		settings:  config.Defaults(),
		locations: paths.Locations{TrashDir: t.TempDir()},
	}
	for _, f := range fakes {
		a.providers = append(a.providers, &providerEntry{Provider: f, Root: t.TempDir()})
	}
	return a
}

// convExchange builds a Conversation with one user prompt
// and one assistant reply at a fixed timestamp. The
// timestamp goes into both message stamps and onto the
// envelope so the rendered Markdown header carries it.
func convExchange(prompt, reply string, ts time.Time) contracts.Conversation {
	return contracts.Conversation{
		StartedAt: ts,
		EndedAt:   ts,
		Messages: []contracts.Message{
			{Role: contracts.RoleUser, Timestamp: ts, Blocks: []contracts.Block{contracts.TextBlock{Text: prompt}}},
			{Role: contracts.RoleAssistant, Timestamp: ts, Blocks: []contracts.Block{contracts.TextBlock{Text: reply}}},
		},
	}
}

// TestBulkExport_callsBackOncePerSessionWithMarkdown is the
// happy path. One provider, one project, two sessions. The
// callback should fire twice and each invocation should
// carry rendered Markdown that includes the prompt text.
func TestBulkExport_callsBackOncePerSessionWithMarkdown(t *testing.T) {
	ts := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	fake := &bulkFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "proj"}},
		summaries: map[contracts.ProjectID][]contracts.SessionSummary{
			"proj": {
				{ID: "s1", Project: "proj", StartedAt: ts, Title: "first"},
				{ID: "s2", Project: "proj", StartedAt: ts.Add(time.Hour), Title: "second"},
			},
		},
		convos: map[contracts.SessionID]contracts.Conversation{
			"s1": convExchange("hello one", "reply one", ts),
			"s2": convExchange("hello two", "reply two", ts.Add(time.Hour)),
		},
	}
	a := makeBulkApp(t, fake)

	var got []BulkExportEntry
	count, err := a.BulkExport("proj", BulkExportOptions{}, func(e BulkExportEntry) error {
		got = append(got, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if len(got) != 2 {
		t.Fatalf("callback fired %d times, want 2", len(got))
	}
	if got[0].SessionID != "s1" || !strings.Contains(got[0].Content, "hello one") {
		t.Errorf("first entry = %+v, want s1 with 'hello one' in content", got[0])
	}
	if got[1].SessionID != "s2" || !strings.Contains(got[1].Content, "hello two") {
		t.Errorf("second entry = %+v, want s2 with 'hello two' in content", got[1])
	}
}

// TestBulkExport_filterOptionsPropagateToRenderer confirms
// the filter knobs reach steps.Filter. We build a session
// with a meta message and a thinking block, ask for both
// to be hidden, and assert the substrings are missing from
// the rendered output.
func TestBulkExport_filterOptionsPropagateToRenderer(t *testing.T) {
	ts := time.Now()
	conv := contracts.Conversation{
		StartedAt: ts,
		Messages: []contracts.Message{
			{
				Role:      contracts.RoleUser,
				IsMeta:    true,
				Timestamp: ts,
				Blocks:    []contracts.Block{contracts.TextBlock{Text: "META-ONLY"}},
			},
			{
				Role:      contracts.RoleAssistant,
				Timestamp: ts.Add(time.Second),
				Blocks: []contracts.Block{
					contracts.ThinkingBlock{Text: "THINK-ONLY"},
					contracts.TextBlock{Text: "real reply"},
				},
			},
		},
	}
	fake := &bulkFake{
		name:      "claude",
		projects:  []contracts.Project{{ID: "p"}},
		summaries: map[contracts.ProjectID][]contracts.SessionSummary{"p": {{ID: "s1", Project: "p"}}},
		convos:    map[contracts.SessionID]contracts.Conversation{"s1": conv},
	}
	a := makeBulkApp(t, fake)

	var captured BulkExportEntry
	_, err := a.BulkExport("p", BulkExportOptions{HideMeta: true, HideThinking: true}, func(e BulkExportEntry) error {
		captured = e
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(captured.Content, "META-ONLY") {
		t.Error("HideMeta did not propagate; meta text leaked into output")
	}
	if strings.Contains(captured.Content, "THINK-ONLY") {
		t.Error("HideThinking did not propagate; thinking text leaked into output")
	}
	if !strings.Contains(captured.Content, "real reply") {
		t.Error("real assistant text was dropped (the filter went too far)")
	}
}

// TestBulkExport_unknownProjectWrapsErrNotExist proves the
// not-found contract. The CLI uses errors.Is to render a
// clean message instead of the raw wrap.
func TestBulkExport_unknownProjectWrapsErrNotExist(t *testing.T) {
	a := makeBulkApp(t, &bulkFake{name: "claude"})
	_, err := a.BulkExport("ghost", BulkExportOptions{}, func(BulkExportEntry) error { return nil })
	if err == nil {
		t.Fatal("expected an error for an unknown project")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want one wrapping fs.ErrNotExist", err)
	}
}

// TestBulkExport_ambiguousProjectReturnsSentinel covers the
// two-providers-same-id case. Without a Provider hint the
// caller cannot know which one to pick, so we surface a
// distinct sentinel for the CLI to translate.
func TestBulkExport_ambiguousProjectReturnsSentinel(t *testing.T) {
	a := makeBulkApp(t,
		&bulkFake{name: "claude", projects: []contracts.Project{{ID: "shared"}}},
		&bulkFake{name: "copilot", projects: []contracts.Project{{ID: "shared"}}},
	)
	_, err := a.BulkExport("shared", BulkExportOptions{}, func(BulkExportEntry) error { return nil })
	if err == nil {
		t.Fatal("expected an error for an ambiguous project id")
	}
	if !errors.Is(err, ErrProjectAmbiguous) {
		t.Errorf("err = %v, want one wrapping ErrProjectAmbiguous", err)
	}
}

// TestBulkExport_providerHintPicksOneSide pins the
// disambiguation contract. With a Provider hint, the
// ambiguous case becomes a clean lookup against the named
// provider.
func TestBulkExport_providerHintPicksOneSide(t *testing.T) {
	ts := time.Now()
	claudeFake := &bulkFake{
		name:      "claude",
		projects:  []contracts.Project{{ID: "shared"}},
		summaries: map[contracts.ProjectID][]contracts.SessionSummary{"shared": {{ID: "claude-s", Project: "shared"}}},
		convos:    map[contracts.SessionID]contracts.Conversation{"claude-s": convExchange("from claude", "ok", ts)},
	}
	copilotFake := &bulkFake{
		name:      "copilot",
		projects:  []contracts.Project{{ID: "shared"}},
		summaries: map[contracts.ProjectID][]contracts.SessionSummary{"shared": {{ID: "copilot-s", Project: "shared"}}},
		convos:    map[contracts.SessionID]contracts.Conversation{"copilot-s": convExchange("from copilot", "ok", ts)},
	}
	a := makeBulkApp(t, claudeFake, copilotFake)

	var got BulkExportEntry
	count, err := a.BulkExport("shared", BulkExportOptions{Provider: "copilot"}, func(e BulkExportEntry) error {
		got = e
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || got.SessionID != "copilot-s" {
		t.Errorf("got = %+v (count=%d), want copilot-s only", got, count)
	}
}

// TestBulkExport_callbackErrorStopsIteration proves a
// failing destination aborts the loop. A disk-full or
// permission error on the third file should not silently
// continue trying to write the rest.
func TestBulkExport_callbackErrorStopsIteration(t *testing.T) {
	ts := time.Now()
	fake := &bulkFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "p"}},
		summaries: map[contracts.ProjectID][]contracts.SessionSummary{
			"p": {
				{ID: "s1", Project: "p"},
				{ID: "s2", Project: "p"},
				{ID: "s3", Project: "p"},
			},
		},
		convos: map[contracts.SessionID]contracts.Conversation{
			"s1": convExchange("a", "b", ts),
			"s2": convExchange("c", "d", ts),
			"s3": convExchange("e", "f", ts),
		},
	}
	a := makeBulkApp(t, fake)

	calls := 0
	stopErr := errors.New("destination unavailable")
	count, err := a.BulkExport("p", BulkExportOptions{}, func(BulkExportEntry) error {
		calls++
		if calls == 2 {
			return stopErr
		}
		return nil
	})
	if !errors.Is(err, stopErr) {
		t.Errorf("err = %v, want it to be stopErr", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (only the first session completed before the second errored)", count)
	}
	if calls != 2 {
		t.Errorf("callback fired %d times, want 2 (third should not have been attempted)", calls)
	}
}

// TestBulkExport_nilCallbackIsErrored covers the defensive
// guard. A nil callback would silently iterate and discard
// every result, which is never what the caller meant to
// do, so we make it loud.
func TestBulkExport_nilCallbackIsErrored(t *testing.T) {
	a := makeBulkApp(t, &bulkFake{name: "claude"})
	_, err := a.BulkExport("p", BulkExportOptions{}, nil)
	if err == nil {
		t.Error("expected an error for a nil callback")
	}
}
