package composition

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/internal/config"
	"github.com/danieljbfz/chronicle/internal/paths"
)

// newTrashTestApp builds an App suitable for trash-subsystem
// tests. It gives the App a real (temporary) trash directory,
// real config defaults, and one fake provider whose Root is a
// real (temporary) data directory. The data directory is what
// the tests populate with fake session files to move into the
// trash.
//
// Returning the data root alongside the App lets the test write
// fixture files at known paths, then assert on the resulting
// trash state.
func newTrashTestApp(t *testing.T, providerName string) (*App, string, string) {
	t.Helper()
	trashRoot := t.TempDir()
	dataRoot := t.TempDir()

	a := &App{
		settings: config.Defaults(),
		locations: paths.Locations{
			TrashDir: trashRoot,
		},
		providers: []*providerEntry{{
			Provider: &fakeNamedProvider{name: providerName},
			Root:     dataRoot,
			FS:       os.DirFS(dataRoot),
		}},
	}
	return a, dataRoot, trashRoot
}

// fakeNamedProvider is a minimal Provider whose only purpose is
// to give tests a Name() value. The trash subsystem does not
// touch any other Provider method, so leaving the rest as no-ops
// is enough.
type fakeNamedProvider struct {
	name string
}

func (f *fakeNamedProvider) Name() string { return f.name }
func (f *fakeNamedProvider) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{}, nil
}
func (f *fakeNamedProvider) ListProjects(fs.FS) ([]contracts.Project, error) {
	return nil, nil
}
func (f *fakeNamedProvider) ListSessionRefs(fs.FS, contracts.ProjectID) ([]contracts.SessionRef, error) {
	return nil, nil
}
func (f *fakeNamedProvider) SummarizeSession(fs.FS, contracts.SessionRef) (contracts.SessionSummary, error) {
	return contracts.SessionSummary{}, nil
}
func (f *fakeNamedProvider) ReadSession(fs.FS, contracts.SessionID) (contracts.Conversation, error) {
	return contracts.Conversation{}, nil
}

// TestTrash_movesFilesAndWritesManifest is the happy-path test.
// We drop one file inside the data root, build a plan that
// targets it, and assert that after Trash the file is no longer
// at its original location, the file is at the expected
// trashed location, and a manifest exists.
func TestTrash_movesFilesAndWritesManifest(t *testing.T) {
	a, dataRoot, trashRoot := newTrashTestApp(t, "claude")

	original := filepath.Join(dataRoot, "projects", "p", "sess.jsonl")
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte(`{"hello":"world"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := contracts.DeletePlan{
		SessionID: "sess",
		Category:  "claude-session",
		Items: []contracts.DeleteItem{{
			Path:      "projects/p/sess.jsonl",
			Reason:    "session file",
			SizeBytes: 17,
		}},
	}
	entry, err := a.Trash(plannedDeletion{provider: a.providers[0], plan: plan})
	if err != nil {
		t.Fatal(err)
	}

	// Original is gone.
	if _, err := os.Stat(original); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("original should be gone, stat err = %v", err)
	}

	// The trashed file lives where the manifest says.
	trashedFile := filepath.Join(trashRoot, entry.ID, filesSubdir, "projects/p/sess.jsonl")
	if _, err := os.Stat(trashedFile); err != nil {
		t.Errorf("trashed file should exist at %s: %v", trashedFile, err)
	}

	// Manifest is readable and matches the entry.
	stored, err := readManifest(filepath.Join(trashRoot, entry.ID))
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}
	if stored.SessionID != "sess" || stored.Provider != "claude" || stored.Category != "claude-session" {
		t.Errorf("manifest fields wrong: %+v", stored)
	}
	if len(stored.Items) != 1 {
		t.Fatalf("manifest items = %d, want 1", len(stored.Items))
	}
	if stored.Items[0].OriginalPath != original {
		t.Errorf("original path = %q, want %q", stored.Items[0].OriginalPath, original)
	}
}

// TestTrash_skipsMissingItems confirms the resilience contract:
// a file the user manually deleted between PlanDelete and Trash
// should be skipped, not abort the whole move. The fixture has
// one real file and one missing file in the same plan. The real
// one should land in trash, and the entry should reflect just
// that one.
func TestTrash_skipsMissingItems(t *testing.T) {
	a, dataRoot, _ := newTrashTestApp(t, "claude")

	real := filepath.Join(dataRoot, "real.txt")
	if err := os.WriteFile(real, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := contracts.DeletePlan{
		Category: "test",
		Items: []contracts.DeleteItem{
			{Path: "real.txt", Reason: "exists"},
			{Path: "missing.txt", Reason: "manually deleted"},
		},
	}
	entry, err := a.Trash(plannedDeletion{provider: a.providers[0], plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Items) != 1 {
		t.Errorf("entry items = %d, want 1 (only the real file)", len(entry.Items))
	}
}

// TestTrashList_returnsNewestFirst proves the listing order is
// stable and useful. Tests create two entries with measurably
// different timestamps and confirm the newer one comes first.
func TestTrashList_returnsNewestFirst(t *testing.T) {
	a, dataRoot, _ := newTrashTestApp(t, "claude")

	for i, name := range []string{"first.txt", "second.txt"} {
		p := filepath.Join(dataRoot, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		plan := contracts.DeletePlan{
			Category: "test",
			Items:    []contracts.DeleteItem{{Path: name}},
		}
		if _, err := a.Trash(plannedDeletion{provider: a.providers[0], plan: plan}); err != nil {
			t.Fatal(err)
		}
		// Sleep long enough that the entry IDs differ even at
		// one-second resolution.
		if i == 0 {
			time.Sleep(1100 * time.Millisecond)
		}
	}

	entries, err := a.TrashList()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if !entries[0].TrashedAt.After(entries[1].TrashedAt) {
		t.Error("entries should be sorted newest first")
	}
}

// TestTrashRestore_movesFilesBack is the round-trip test: trash
// a file, then restore it, then assert it is back at its
// original path and the trash entry is gone. This is the path
// the user takes when they realize they trashed the wrong
// thing.
func TestTrashRestore_movesFilesBack(t *testing.T) {
	a, dataRoot, trashRoot := newTrashTestApp(t, "claude")

	original := filepath.Join(dataRoot, "important.txt")
	if err := os.WriteFile(original, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := contracts.DeletePlan{
		Category: "test",
		Items:    []contracts.DeleteItem{{Path: "important.txt"}},
	}
	entry, err := a.Trash(plannedDeletion{provider: a.providers[0], plan: plan})
	if err != nil {
		t.Fatal(err)
	}

	if err := a.TrashRestore(entry.ID); err != nil {
		t.Fatal(err)
	}

	// Original is back.
	got, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("original should be back: %v", err)
	}
	if string(got) != "keep me" {
		t.Errorf("restored content = %q, want %q", got, "keep me")
	}
	// Trash entry is gone.
	if _, err := os.Stat(filepath.Join(trashRoot, entry.ID)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("trash entry should be gone after restore, stat err = %v", err)
	}
}

// TestTrashRestore_refusesToOverwrite proves the safety check:
// if the original path now contains different data, restore
// must refuse. Otherwise we would silently destroy whatever the
// user wrote there in the meantime.
func TestTrashRestore_refusesToOverwrite(t *testing.T) {
	a, dataRoot, _ := newTrashTestApp(t, "claude")

	original := filepath.Join(dataRoot, "file.txt")
	if err := os.WriteFile(original, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := contracts.DeletePlan{
		Category: "test",
		Items:    []contracts.DeleteItem{{Path: "file.txt"}},
	}
	entry, err := a.Trash(plannedDeletion{provider: a.providers[0], plan: plan})
	if err != nil {
		t.Fatal(err)
	}

	// Recreate the file at the original path with different
	// content, simulating the user writing new data.
	if err := os.WriteFile(original, []byte("v2 - different"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := a.TrashRestore(entry.ID); err == nil {
		t.Error("restore should refuse to overwrite an existing file")
	}
}

// TestTrashRestore_abortsWhenADestinationCannotBeChecked pins the
// pre-flight against an ambiguous stat. The pre-flight is meant to
// clear every destination before moving any item, so a restore
// either fully happens or does not start. When a stat fails for a
// reason other than "not found" — here a destination whose parent
// is a regular file, which yields ENOTDIR — the restore must abort
// before moving anything. The fixture trashes two items, then turns
// the second item's parent into a file. The first item must remain
// in the trash, proving the restore did not half-complete.
func TestTrashRestore_abortsWhenADestinationCannotBeChecked(t *testing.T) {
	a, dataRoot, _ := newTrashTestApp(t, "claude")

	clean := filepath.Join(dataRoot, "clean.txt")
	if err := os.WriteFile(clean, []byte("restore me"), 0o644); err != nil {
		t.Fatal(err)
	}
	blockedDir := filepath.Join(dataRoot, "blocker")
	if err := os.Mkdir(blockedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blockedDir, "inner.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := contracts.DeletePlan{
		Category: "test",
		Items: []contracts.DeleteItem{
			{Path: "clean.txt"},
			{Path: "blocker/inner.txt"},
		},
	}
	entry, err := a.Trash(plannedDeletion{provider: a.providers[0], plan: plan})
	if err != nil {
		t.Fatal(err)
	}

	// Replace the emptied blocker directory with a regular file, so
	// stat-ing the second item's destination fails with ENOTDIR
	// rather than a clean "not found".
	if err := os.Remove(blockedDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blockedDir, []byte("now a file"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := a.TrashRestore(entry.ID); err == nil {
		t.Fatal("restore should abort when a destination cannot be stat-checked")
	}

	// The first item must not have been restored: the pre-flight
	// aborted before any move, so a clean restore did not leak out.
	if _, err := os.Stat(clean); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("first item should stay in the trash after an aborted restore, stat err = %v", err)
	}
}

// TestTrashEmpty_respectsRetentionWindow proves the
// retention-window guard. With the default 30-day retention,
// entries created just now should not be removed. Pass a Now
// far in the future to confirm they would be removed once they
// age out.
func TestTrashEmpty_respectsRetentionWindow(t *testing.T) {
	a, dataRoot, trashRoot := newTrashTestApp(t, "claude")

	if err := os.WriteFile(filepath.Join(dataRoot, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := contracts.DeletePlan{
		Category: "test",
		Items:    []contracts.DeleteItem{{Path: "x.txt"}},
	}
	entry, err := a.Trash(plannedDeletion{provider: a.providers[0], plan: plan})
	if err != nil {
		t.Fatal(err)
	}

	// Default retention (30 days) should keep the entry.
	removed, err := a.TrashEmpty(TrashEmptyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Errorf("removed %d entries, want 0 (within retention window)", len(removed))
	}
	if _, err := os.Stat(filepath.Join(trashRoot, entry.ID)); err != nil {
		t.Errorf("entry should still exist: %v", err)
	}

	// Time-travel forward 60 days. The entry is now well past
	// its retention window, so empty removes it.
	future := time.Now().UTC().Add(60 * 24 * time.Hour)
	removed, err = a.TrashEmpty(TrashEmptyOptions{Now: future})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != entry.ID {
		t.Errorf("removed = %v, want [%s]", removed, entry.ID)
	}
}

// TestTrashEmpty_forceRemovesEverything proves the Force flag
// bypasses the retention window. The user runs `chronicle
// trash empty --force` when they really do want to clear the
// trash now.
func TestTrashEmpty_forceRemovesEverything(t *testing.T) {
	a, dataRoot, _ := newTrashTestApp(t, "claude")

	if err := os.WriteFile(filepath.Join(dataRoot, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := contracts.DeletePlan{
		Category: "test",
		Items:    []contracts.DeleteItem{{Path: "x.txt"}},
	}
	if _, err := a.Trash(plannedDeletion{provider: a.providers[0], plan: plan}); err != nil {
		t.Fatal(err)
	}

	removed, err := a.TrashEmpty(TrashEmptyOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 {
		t.Errorf("removed %d entries, want 1 (force ignores retention)", len(removed))
	}
}

// TestNewTrashEntryID_isUniqueAndSortable confirms the ID
// generator produces sortable, distinct values even for
// timestamps in the same second. Without the random suffix,
// two near-simultaneous trash operations would collide.
func TestNewTrashEntryID_isUniqueAndSortable(t *testing.T) {
	now := time.Date(2026, 5, 15, 10, 30, 45, 0, time.UTC)
	a, errA := newTrashEntryID(now)
	b, errB := newTrashEntryID(now)
	if errA != nil || errB != nil {
		t.Fatalf("errA=%v errB=%v", errA, errB)
	}
	if a == b {
		t.Errorf("two same-second IDs collided: %q", a)
	}
	// Both should sort under a future timestamp.
	later, _ := newTrashEntryID(now.Add(time.Hour))
	if !(a < later && b < later) {
		t.Errorf("IDs are not chronologically sortable: %q %q vs %q", a, b, later)
	}
}
