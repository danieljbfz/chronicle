package composition

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/internal/config"
	"github.com/danieljbfz/chronicle/internal/paths"
)

// resumableFake implements both Provider and Resumable. The
// known map holds the session IDs the provider owns and
// what cwd to return for each. The fake never reads from
// the fs.FS argument because resume composition does not
// hand it any meaningful filesystem in tests.
type resumableFake struct {
	name  string
	known map[contracts.SessionID]string
}

func (f *resumableFake) Name() string { return f.name }
func (f *resumableFake) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{}, nil
}
func (f *resumableFake) ListProjects(fs.FS) ([]contracts.Project, error) { return nil, nil }
func (f *resumableFake) ListSessionRefs(fs.FS, contracts.ProjectID) ([]contracts.SessionRef, error) {
	return nil, nil
}
func (f *resumableFake) SummarizeSession(fs.FS, contracts.SessionRef) (contracts.SessionSummary, error) {
	return contracts.SessionSummary{}, nil
}
func (f *resumableFake) ReadSession(_ fs.FS, id contracts.SessionID) (contracts.Conversation, error) {
	if _, ok := f.known[id]; !ok {
		return contracts.Conversation{}, fs.ErrNotExist
	}
	return contracts.Conversation{}, nil
}
func (f *resumableFake) ResumeCommand(_ fs.FS, id contracts.SessionID) (contracts.ResumePlan, error) {
	cwd, ok := f.known[id]
	if !ok {
		return contracts.ResumePlan{}, fs.ErrNotExist
	}
	return contracts.ResumePlan{
		Command:    []string{f.name, "--resume", string(id)},
		WorkingDir: cwd,
	}, nil
}

// nonResumableFake implements Provider only. Composition
// has to fall through with ErrResumeUnsupported when a
// session lives in this kind of provider.
type nonResumableFake struct {
	name  string
	known map[contracts.SessionID]bool
}

func (f *nonResumableFake) Name() string { return f.name }
func (f *nonResumableFake) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{}, nil
}
func (f *nonResumableFake) ListProjects(fs.FS) ([]contracts.Project, error) { return nil, nil }
func (f *nonResumableFake) ListSessionRefs(fs.FS, contracts.ProjectID) ([]contracts.SessionRef, error) {
	return nil, nil
}
func (f *nonResumableFake) SummarizeSession(fs.FS, contracts.SessionRef) (contracts.SessionSummary, error) {
	return contracts.SessionSummary{}, nil
}
func (f *nonResumableFake) ReadSession(_ fs.FS, id contracts.SessionID) (contracts.Conversation, error) {
	if !f.known[id] {
		return contracts.Conversation{}, fs.ErrNotExist
	}
	return contracts.Conversation{}, nil
}

// makeResumeApp wires fakes (any combination of resumable
// and non-resumable) into an App. Tests pass them in
// registration order so the App walks them in that order
// when looking up a session.
func makeResumeApp(t *testing.T, providers ...contracts.Provider) *App {
	t.Helper()
	a := &App{
		settings:  config.Defaults(),
		locations: paths.Locations{TrashDir: t.TempDir()},
	}
	for _, p := range providers {
		a.providers = append(a.providers, &providerEntry{Provider: p, Root: t.TempDir()})
	}
	return a
}

// TestResume_returnsPlanFromResumableProvider is the happy
// path. One provider owns the session and is resumable,
// composition returns the wrapped plan with the provider
// name attached.
func TestResume_returnsPlanFromResumableProvider(t *testing.T) {
	fake := &resumableFake{
		name:  "claude",
		known: map[contracts.SessionID]string{"s1": "/Users/x/work/foo"},
	}
	a := makeResumeApp(t, fake)

	got, err := a.Resume("s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "claude" {
		t.Errorf("provider = %q, want claude", got.Provider)
	}
	if got.Plan.WorkingDir != "/Users/x/work/foo" {
		t.Errorf("working dir = %q, want /Users/x/work/foo", got.Plan.WorkingDir)
	}
	if len(got.Plan.Command) == 0 || got.Plan.Command[0] != "claude" {
		t.Errorf("command = %v, want first element 'claude'", got.Plan.Command)
	}
}

// TestResume_unknownSessionReturnsNotExist confirms the
// fall-through behaviour. Two providers, neither owns the
// session, the result wraps fs.ErrNotExist so the CLI can
// distinguish "missing" from other failure modes.
func TestResume_unknownSessionReturnsNotExist(t *testing.T) {
	a := makeResumeApp(t,
		&resumableFake{name: "claude"},
		&nonResumableFake{name: "copilot"},
	)
	_, err := a.Resume("ghost")
	if err == nil {
		t.Fatal("expected an error for an unknown session id")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want one wrapping fs.ErrNotExist", err)
	}
}

// TestResume_nonResumableProviderReturnsClearError covers
// the "Copilot session" scenario. The provider owns the
// session but does not implement Resumable, so composition
// has to surface ErrResumeUnsupported with the provider
// name embedded in the wrap so the CLI can name it.
func TestResume_nonResumableProviderReturnsClearError(t *testing.T) {
	a := makeResumeApp(t, &nonResumableFake{
		name:  "copilot",
		known: map[contracts.SessionID]bool{"s1": true},
	})

	_, err := a.Resume("s1")
	if err == nil {
		t.Fatal("expected an error for a non-resumable provider")
	}
	if !errors.Is(err, ErrResumeUnsupported) {
		t.Errorf("err = %v, want one wrapping ErrResumeUnsupported", err)
	}
	if !strings.Contains(err.Error(), "copilot") {
		t.Errorf("err = %q, want it to mention the provider name", err)
	}
}

// TestResume_walksProvidersInRegistrationOrder pins the
// lookup order. Two resumable providers, both happen to
// know the same session id (a synthetic situation, but a
// reasonable behaviour to lock in). The first registered
// provider wins, so the result identifies it.
func TestResume_walksProvidersInRegistrationOrder(t *testing.T) {
	first := &resumableFake{
		name:  "first",
		known: map[contracts.SessionID]string{"s1": "/from/first"},
	}
	second := &resumableFake{
		name:  "second",
		known: map[contracts.SessionID]string{"s1": "/from/second"},
	}
	a := makeResumeApp(t, first, second)

	got, err := a.Resume("s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "first" {
		t.Errorf("provider = %q, want first (registration order)", got.Provider)
	}
	if got.Plan.WorkingDir != "/from/first" {
		t.Errorf("working dir = %q, want /from/first", got.Plan.WorkingDir)
	}
}
