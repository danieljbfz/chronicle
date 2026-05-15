package composition

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/BurntSushi/toml"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/internal/config"
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

// TestDoctor_addsWarningForUnknownVersion proves the doctor view
// TestSettingsTOML_roundTripsThroughTheDecoder pins the
// most important property of the rendered output: a user
// who pipes `chronicle config show` back into their config
// file should get the same Config back. The test renders
// the defaults, decodes the result, and confirms the trash
// retention value (a representative scalar) and the
// providers map (the part the audit pass refactored)
// survive the round trip.
func TestSettingsTOML_roundTripsThroughTheDecoder(t *testing.T) {
	a := newAppWithFakes(&fakeProvider{name: "claude"})
	a.settings = config.Defaults()
	rendered, err := a.SettingsTOML()
	if err != nil {
		t.Fatal(err)
	}
	if rendered == "" {
		t.Fatal("SettingsTOML returned an empty string")
	}

	var decoded config.Config
	if _, err := toml.Decode(rendered, &decoded); err != nil {
		t.Fatalf("rendered TOML did not decode: %v\n---\n%s", err, rendered)
	}
	if decoded.Trash.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30", decoded.Trash.RetentionDays)
	}
	if !decoded.Providers[config.ProviderClaude].Enabled {
		t.Error("Claude should be enabled in the round-tripped config")
	}
	if !decoded.Providers[config.ProviderCopilot].Enabled {
		t.Error("Copilot should be enabled in the round-tripped config")
	}
}

// records a warning when the storage version is unknown. The user
// reads warnings in the doctor output, and the cleanup commands
// use the same fact (Version == "unknown") to require an extra
// confirmation before doing anything destructive.
func TestDoctor_addsWarningForUnknownVersion(t *testing.T) {
	a := newAppWithFakes(&fakeProvider{
		name:    "claude",
		version: contracts.StorageVersion{Adapter: "claude", Version: "unknown", Fingerprint: "deadbeef"},
	})
	healths := a.Doctor()
	if len(healths) != 1 {
		t.Fatalf("got %d health entries, want 1", len(healths))
	}
	if len(healths[0].Warnings) == 0 {
		t.Error("Doctor should record a warning for unknown version")
	}
	if len(healths[0].Errors) != 0 {
		t.Errorf("unknown version is a warning, not an error: %v", healths[0].Errors)
	}
}
