package composition

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/internal/config"
	"github.com/danieljbfz/chronicle/internal/paths"
)

// memoryFake is a Provider that also implements MemoryStore.
// We use it inside the memory tests to exercise composition's
// memory orchestration without touching the Claude adapter
// directly. The composition layer only sees the contract
// interface, so a fake that satisfies the same interface is
// faithful enough.
//
// The fake stores the per-project memory files as a map
// keyed by project, with each value being the list of files
// in that project's memory directory. The tests populate the
// map up front and then call composition methods against it.
type memoryFake struct {
	name  string
	files map[contracts.ProjectID][]contracts.MemoryFile
	// missingProject is the project ID that triggers an
	// fs.ErrNotExist from PlanDeleteProjectMemory. The
	// production Claude adapter returns this error when the
	// project has no memory directory at all.
	missingProject contracts.ProjectID
}

func (f *memoryFake) Name() string { return f.name }
func (f *memoryFake) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{}, nil
}
func (f *memoryFake) ListProjects(fs.FS) ([]contracts.Project, error) {
	return nil, nil
}
func (f *memoryFake) ListSessions(fs.FS, contracts.ProjectID) ([]contracts.SessionSummary, error) {
	return nil, nil
}
func (f *memoryFake) ReadSession(fs.FS, contracts.SessionID) (contracts.Conversation, error) {
	return contracts.Conversation{}, nil
}

// ListMemories returns every memory file the fake has been
// configured with, across every project. The result mirrors
// the contract: it is empty when no project has memory, and
// it is sorted in a stable order so tests can pin positions.
func (f *memoryFake) ListMemories(fs.FS) ([]contracts.MemoryFile, error) {
	var out []contracts.MemoryFile
	for _, files := range f.files {
		out = append(out, files...)
	}
	return out, nil
}

// MemoryFilePath builds the same path shape the Claude
// adapter uses, so the file-existence checks in composition
// resolve the right disk locations.
func (f *memoryFake) MemoryFilePath(project contracts.ProjectID, fileName string) string {
	return filepath.Join("memory", string(project), fileName)
}

// PlanDeleteProjectMemory returns a plan with one item per
// memory file in the project, or an fs.ErrNotExist when the
// project is the one the test configured as missing.
func (f *memoryFake) PlanDeleteProjectMemory(_ fs.FS, project contracts.ProjectID) (contracts.DeletePlan, error) {
	if project == f.missingProject {
		return contracts.DeletePlan{}, fs.ErrNotExist
	}
	plan := contracts.DeletePlan{Category: "fake-memory"}
	for _, file := range f.files[project] {
		plan.Items = append(plan.Items, contracts.DeleteItem{
			Path:   f.MemoryFilePath(project, file.FileName),
			Reason: "memory file",
		})
	}
	return plan, nil
}

// newMemoryTestApp builds an App with one memoryFake
// provider plus a real (temporary) data root and trash
// directory. The data root is real because composition's
// memory methods read and write actual files; the fake
// stands in only for the provider interface methods.
func newMemoryTestApp(t *testing.T, fake *memoryFake) (*App, string) {
	t.Helper()
	dataRoot := t.TempDir()
	a := &App{
		settings:  config.Defaults(),
		locations: paths.Locations{TrashDir: t.TempDir()},
		providers: []*providerEntry{{
			Provider: fake,
			Root:     dataRoot,
			FS:       os.DirFS(dataRoot),
		}},
	}
	return a, dataRoot
}

// TestListMemories_returnsEntriesFromMemoryStoreProvider
// confirms the happy path. With one memory-capable provider
// and two memory files, ListMemories returns two listings
// with the right fields populated.
func TestListMemories_returnsEntriesFromMemoryStoreProvider(t *testing.T) {
	fake := &memoryFake{
		name: "claude",
		files: map[contracts.ProjectID][]contracts.MemoryFile{
			"proj-a": {
				{Project: "proj-a", FileName: "MEMORY.md", SizeBytes: 100, ModifiedAt: time.Now()},
				{Project: "proj-a", FileName: "architecture.md", SizeBytes: 250, ModifiedAt: time.Now()},
			},
		},
	}
	a, _ := newMemoryTestApp(t, fake)

	listings, err := a.ListMemories()
	if err != nil {
		t.Fatal(err)
	}
	if len(listings) != 2 {
		t.Fatalf("listings = %d, want 2", len(listings))
	}
	if listings[0].Provider != "claude" {
		t.Errorf("provider = %q, want claude", listings[0].Provider)
	}
	if listings[0].SizeBytes == 0 {
		t.Error("size should be non-zero")
	}
}

// TestListMemories_emptyWhenNoMemoryStoreProvider confirms
// the empty-result path. A provider that does not implement
// MemoryStore contributes nothing, and the function returns
// an empty slice instead of an error.
func TestListMemories_emptyWhenNoMemoryStoreProvider(t *testing.T) {
	a := &App{
		settings:  config.Defaults(),
		locations: paths.Locations{TrashDir: t.TempDir()},
		providers: []*providerEntry{
			{Provider: &readOnlyFake{}, Root: t.TempDir()},
		},
	}
	listings, err := a.ListMemories()
	if err != nil {
		t.Fatal(err)
	}
	if len(listings) != 0 {
		t.Errorf("listings = %d, want 0 for a non-memory provider", len(listings))
	}
}

// TestShowMemory_writesFileContentsToWriter confirms the
// show path round-trips one memory file from disk to a
// writer. The CLI uses this to print memory content to
// stdout.
func TestShowMemory_writesFileContentsToWriter(t *testing.T) {
	fake := &memoryFake{
		name: "claude",
		files: map[contracts.ProjectID][]contracts.MemoryFile{
			"proj-a": {{Project: "proj-a", FileName: "MEMORY.md"}},
		},
	}
	a, dataRoot := newMemoryTestApp(t, fake)

	// Drop the file at the path the fake's MemoryFilePath
	// returns. The full path is dataRoot + "memory/proj-a/MEMORY.md".
	rel := fake.MemoryFilePath("proj-a", "MEMORY.md")
	full := filepath.Join(dataRoot, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	want := "# Index\n\n- Architecture: see architecture.md\n"
	if err := os.WriteFile(full, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := a.ShowMemory("proj-a", "MEMORY.md", &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != want {
		t.Errorf("ShowMemory wrote %q, want %q", buf.String(), want)
	}
}

// TestShowMemory_missingFileReturnsError confirms the
// not-found path. The CLI relies on the error type to print
// "no such memory file" instead of dumping a raw read
// failure.
func TestShowMemory_missingFileReturnsError(t *testing.T) {
	fake := &memoryFake{name: "claude"}
	a, _ := newMemoryTestApp(t, fake)

	var buf bytes.Buffer
	err := a.ShowMemory("proj-a", "missing.md", &buf)
	if err == nil {
		t.Fatal("expected an error for a missing memory file")
	}
}

// TestEditMemoryPath_returnsAbsolutePathWhenFileExists
// confirms the happy path for the edit command. When the
// file exists on disk, the function returns the absolute
// path the CLI can hand to $EDITOR.
func TestEditMemoryPath_returnsAbsolutePathWhenFileExists(t *testing.T) {
	fake := &memoryFake{name: "claude"}
	a, dataRoot := newMemoryTestApp(t, fake)

	rel := fake.MemoryFilePath("proj-a", "MEMORY.md")
	full := filepath.Join(dataRoot, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("# memory"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := a.EditMemoryPath("proj-a", "MEMORY.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != full {
		t.Errorf("EditMemoryPath = %q, want %q", got, full)
	}
}

// TestEditMemoryPath_refusesMissingFile pins the safety
// check. Opening a missing file in $EDITOR would create an
// empty new file on save, which is not what the user asked
// for. The function refuses up front so the CLI can print a
// clear error.
func TestEditMemoryPath_refusesMissingFile(t *testing.T) {
	fake := &memoryFake{name: "claude"}
	a, _ := newMemoryTestApp(t, fake)

	_, err := a.EditMemoryPath("proj-a", "missing.md")
	if err == nil {
		t.Error("expected an error for a missing memory file")
	}
}

// TestCleanProjectMemory_returnsPlanReadyForExecution
// confirms the clean-memory path builds a PlannedDeletion
// the caller can pass to ExecuteCleanup. The shape mirrors
// what the abandoned-cleanup category returns, so the same
// rendering code in the CLI handles both.
func TestCleanProjectMemory_returnsPlanReadyForExecution(t *testing.T) {
	fake := &memoryFake{
		name: "claude",
		files: map[contracts.ProjectID][]contracts.MemoryFile{
			"proj-a": {
				{Project: "proj-a", FileName: "MEMORY.md"},
				{Project: "proj-a", FileName: "architecture.md"},
			},
		},
	}
	a, _ := newMemoryTestApp(t, fake)

	planned, err := a.CleanProjectMemory("proj-a")
	if err != nil {
		t.Fatal(err)
	}
	if planned.ProviderName() != "claude" {
		t.Errorf("provider = %q, want claude", planned.ProviderName())
	}
	if len(planned.Plan.Items) != 2 {
		t.Errorf("items = %d, want 2", len(planned.Plan.Items))
	}
}

// TestCleanProjectMemory_missingProjectReturnsError pins
// the not-found behaviour. The error should surface so the
// CLI can print "no memory for that project" instead of
// silently producing an empty plan that the user would
// misread as "nothing to clean."
func TestCleanProjectMemory_missingProjectReturnsError(t *testing.T) {
	fake := &memoryFake{name: "claude", missingProject: "no-such-project"}
	a, _ := newMemoryTestApp(t, fake)

	_, err := a.CleanProjectMemory("no-such-project")
	if err == nil {
		t.Error("expected an error for a project with no memory")
	}
}

// TestMemoryOperations_failWhenNoMemoryStoreRegistered
// confirms the three memory operations return a clear error
// when no registered provider implements MemoryStore. The
// error wording is part of the CLI surface, so we assert
// the substring users will see.
func TestMemoryOperations_failWhenNoMemoryStoreRegistered(t *testing.T) {
	a := &App{
		settings:  config.Defaults(),
		locations: paths.Locations{TrashDir: t.TempDir()},
		providers: []*providerEntry{
			{Provider: &readOnlyFake{}, Root: t.TempDir()},
		},
	}

	var buf bytes.Buffer
	if err := a.ShowMemory("proj-a", "MEMORY.md", &buf); err == nil {
		t.Error("ShowMemory should fail without a memory provider")
	} else if !strings.Contains(err.Error(), "no registered provider") {
		t.Errorf("ShowMemory error = %v, want one mentioning the missing provider", err)
	}

	if _, err := a.EditMemoryPath("proj-a", "MEMORY.md"); err == nil {
		t.Error("EditMemoryPath should fail without a memory provider")
	}

	if _, err := a.CleanProjectMemory("proj-a"); err == nil {
		t.Error("CleanProjectMemory should fail without a memory provider")
	}
}
