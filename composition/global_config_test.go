package composition

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/internal/config"
	"github.com/danieljbfz/chronicle/internal/paths"
)

// globalConfigFake satisfies both Provider and the
// optional GlobalConfig capability. The fake records every
// RemoveConfigProjectEntries call so the tests can assert
// on which keys reached the provider and in what bucket.
// We keep one fake type per capability, mirroring the
// pattern the memory tests use, because the composition
// layer also keeps each capability separate.
type globalConfigFake struct {
	name        string
	entries     []contracts.ConfigProjectEntry
	removed     []string
	listError   error
	removeError error
}

func (f *globalConfigFake) Name() string { return f.name }
func (f *globalConfigFake) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{}, nil
}
func (f *globalConfigFake) ListProjects(fs.FS) ([]contracts.Project, error) { return nil, nil }
func (f *globalConfigFake) ListSessions(fs.FS, contracts.ProjectID) ([]contracts.SessionSummary, error) {
	return nil, nil
}
func (f *globalConfigFake) ReadSession(fs.FS, contracts.SessionID) (contracts.Conversation, error) {
	return contracts.Conversation{}, nil
}
func (f *globalConfigFake) ListConfigProjectEntries(fs.FS) ([]contracts.ConfigProjectEntry, error) {
	if f.listError != nil {
		return nil, f.listError
	}
	return f.entries, nil
}
func (f *globalConfigFake) RemoveConfigProjectEntries(_ fs.FS, keys []string) (string, error) {
	if f.removeError != nil {
		return "", f.removeError
	}
	f.removed = append(f.removed, keys...)
	return "/tmp/fake-backup", nil
}

// makeGlobalConfigApp wires one or more globalConfigFakes
// into an App, mirroring the helpers the other composition
// tests use.
func makeGlobalConfigApp(t *testing.T, fakes ...*globalConfigFake) *App {
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

// TestListConfigProjects_returnsEntriesFromOneProvider is
// the happy path. One provider, two entries (one stale,
// one live). The listing should carry both with the
// provider name attached.
func TestListConfigProjects_returnsEntriesFromOneProvider(t *testing.T) {
	fake := &globalConfigFake{
		name: "claude",
		entries: []contracts.ConfigProjectEntry{
			{Key: "/Users/x/keep", Exists: true, SizeBytes: 100},
			{Key: "/Users/x/gone", Exists: false, SizeBytes: 50},
		},
	}
	a := makeGlobalConfigApp(t, fake)

	listings, err := a.ListConfigProjects("")
	if err != nil {
		t.Fatal(err)
	}
	if len(listings) != 2 {
		t.Fatalf("listings = %d, want 2", len(listings))
	}
	for _, l := range listings {
		if l.Provider != "claude" {
			t.Errorf("provider = %q, want claude", l.Provider)
		}
	}
}

// TestListConfigProjects_filtersByProvider mirrors the
// pattern used by the other listing methods. Two providers
// with global-config support, and the filter should scope
// the result to the named one.
func TestListConfigProjects_filtersByProvider(t *testing.T) {
	a := makeGlobalConfigApp(t,
		&globalConfigFake{name: "claude", entries: []contracts.ConfigProjectEntry{{Key: "/c/keep"}}},
		&globalConfigFake{name: "cursor", entries: []contracts.ConfigProjectEntry{{Key: "/cu/keep"}}},
	)

	listings, err := a.ListConfigProjects("cursor")
	if err != nil {
		t.Fatal(err)
	}
	if len(listings) != 1 {
		t.Fatalf("listings = %d, want 1", len(listings))
	}
	if listings[0].Provider != "cursor" {
		t.Errorf("provider = %q, want cursor", listings[0].Provider)
	}
}

// TestListConfigProjects_skipsProvidersWithoutCapability
// confirms the optional-capability discovery. A read-only
// provider contributes nothing. Without this guard, the
// composition layer would have to know which adapters
// implement which capability, which defeats the whole
// optional-interface design.
func TestListConfigProjects_skipsProvidersWithoutCapability(t *testing.T) {
	a := &App{
		settings:  config.Defaults(),
		locations: paths.Locations{TrashDir: t.TempDir()},
		providers: []*providerEntry{
			{Provider: &readOnlyFake{}, Root: t.TempDir()},
		},
	}
	listings, err := a.ListConfigProjects("")
	if err != nil {
		t.Fatal(err)
	}
	if len(listings) != 0 {
		t.Errorf("listings = %d, want 0 for a non-GlobalConfig provider", len(listings))
	}
}

// TestCleanConfigProjects_dispatchesByProvider proves the
// bucketing. Two providers, three removals split across
// them. Each provider should receive exactly its own keys,
// not the other provider's.
func TestCleanConfigProjects_dispatchesByProvider(t *testing.T) {
	claudeFake := &globalConfigFake{name: "claude"}
	cursorFake := &globalConfigFake{name: "cursor"}
	a := makeGlobalConfigApp(t, claudeFake, cursorFake)

	results, err := a.CleanConfigProjects([]ConfigProjectListing{
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{Key: "/c/a"}},
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{Key: "/c/b"}},
		{Provider: "cursor", Entry: contracts.ConfigProjectEntry{Key: "/cu/x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if got, want := claudeFake.removed, []string{"/c/a", "/c/b"}; !equalSlices(got, want) {
		t.Errorf("claude received %v, want %v", got, want)
	}
	if got, want := cursorFake.removed, []string{"/cu/x"}; !equalSlices(got, want) {
		t.Errorf("cursor received %v, want %v", got, want)
	}
}

// TestCleanConfigProjects_emptyInputIsNoOp pins the
// shortcut. Nothing to remove means nothing to call.
func TestCleanConfigProjects_emptyInputIsNoOp(t *testing.T) {
	fake := &globalConfigFake{name: "claude"}
	a := makeGlobalConfigApp(t, fake)
	results, err := a.CleanConfigProjects(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("results = %d, want 0", len(results))
	}
	if len(fake.removed) != 0 {
		t.Errorf("provider received removals despite empty input: %v", fake.removed)
	}
}

// TestCleanConfigProjects_propagatesProviderError covers
// the failure path. The provider's removal returns an
// error, and the composition layer wraps it without
// dropping the partial results.
func TestCleanConfigProjects_propagatesProviderError(t *testing.T) {
	fake := &globalConfigFake{name: "claude", removeError: errors.New("disk on fire")}
	a := makeGlobalConfigApp(t, fake)
	_, err := a.CleanConfigProjects([]ConfigProjectListing{
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{Key: "/c/a"}},
	})
	if err == nil {
		t.Fatal("expected an error from the failing provider")
	}
}

// equalSlices is a small helper for the slice-equality
// assertions above. We keep it local to this test file
// because the rest of the composition tests do their own
// equality checks inline.
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
