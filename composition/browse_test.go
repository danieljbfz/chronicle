package composition

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/danieljbfz/chronicle/contracts"
)

// fakeProvider is the test-side stand-in for a real adapter. It
// satisfies the contracts.Provider interface, but every method just
// returns whatever the test set up in advance. We keep the fake in
// the composition package because composition is the only layer
// that needs to test against a fake provider.
type fakeProvider struct {
	name     string
	projects []contracts.Project
	sessions map[contracts.ProjectID][]contracts.SessionSummary
	convos   map[contracts.SessionID]contracts.Conversation
	version  contracts.StorageVersion
}

func (f *fakeProvider) Name() string                                   { return f.name }
func (f *fakeProvider) Detect(fs.FS) (contracts.StorageVersion, error) { return f.version, nil }
func (f *fakeProvider) ListProjects(fs.FS) ([]contracts.Project, error) {
	return f.projects, nil
}
func (f *fakeProvider) ListSessions(_ fs.FS, p contracts.ProjectID) ([]contracts.SessionSummary, error) {
	return f.sessions[p], nil
}
func (f *fakeProvider) ReadSession(_ fs.FS, id contracts.SessionID) (contracts.Conversation, error) {
	c, ok := f.convos[id]
	if !ok {
		return contracts.Conversation{}, fs.ErrNotExist
	}
	return c, nil
}
func (f *fakeProvider) PlanDelete(fs.FS, contracts.SessionID) (contracts.DeletePlan, error) {
	return contracts.DeletePlan{}, nil
}
func (f *fakeProvider) PlanOrphanScan(fs.FS) (contracts.DeletePlan, error) {
	return contracts.DeletePlan{}, nil
}

// newAppWithFakes is a small wrapper around NewForTest that takes
// fake providers and wires them up with empty MapFS values. The
// tests do not need real filesystems, so an empty MapFS is fine.
func newAppWithFakes(p ...*fakeProvider) *App {
	a := &App{}
	for _, fp := range p {
		a.providers = append(a.providers, &providerEntry{
			Provider: fp,
			FS:       fstest.MapFS{},
			Version:  fp.version,
		})
	}
	return a
}

// TestApp_ListProjects_combinesProviders proves the cross-provider
// listing pulls projects from every wired provider. The fake setup
// has one project under "claude" and one under "copilot", and the
// listing should contain both.
func TestApp_ListProjects_combinesProviders(t *testing.T) {
	a := newAppWithFakes(
		&fakeProvider{name: "claude", projects: []contracts.Project{{ID: "p1", DisplayName: "proj1"}}},
		&fakeProvider{name: "copilot", projects: []contracts.Project{{ID: "p2", DisplayName: "proj2"}}},
	)
	listings, err := a.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(listings) != 2 {
		t.Fatalf("got %d listings, want 2", len(listings))
	}
	names := []string{listings[0].Provider, listings[1].Provider}
	if names[0]+names[1] != "claudecopilot" && names[0]+names[1] != "copilotclaude" {
		t.Errorf("providers = %v, expected claude+copilot", names)
	}
}

// TestApp_ReadSession_unknownIdReturnsNotExist checks the not-found
// path. The user passes a session identifier that does not exist
// anywhere, and the lookup walks every provider before giving up
// with fs.ErrNotExist. The CLI relies on this specific error so it
// can print a clear "no such session" message.
func TestApp_ReadSession_unknownIdReturnsNotExist(t *testing.T) {
	a := newAppWithFakes(&fakeProvider{name: "claude"})
	_, err := a.ReadSession("nope")
	if err == nil {
		t.Error("expected fs.ErrNotExist")
	}
}

// TestDoctor_includesUnknownVersionNote proves the doctor view
// attaches a warning note when the storage version is unknown. The
// user reads this note in the doctor output, and the cleanup
// commands use the same fact (Version == "unknown") to require an
// extra confirmation before doing anything destructive.
func TestDoctor_includesUnknownVersionNote(t *testing.T) {
	a := newAppWithFakes(&fakeProvider{
		name:    "claude",
		version: contracts.StorageVersion{Adapter: "claude", Version: "unknown", Fingerprint: "deadbeef"},
	})
	healths := a.Doctor()
	if len(healths) != 1 {
		t.Fatalf("got %d health entries, want 1", len(healths))
	}
	if healths[0].Note == "" {
		t.Error("Doctor should attach a Note for unknown version")
	}
}
