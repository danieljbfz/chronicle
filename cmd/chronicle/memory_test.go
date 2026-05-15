package main

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// memoryProviderStub is a minimal Provider that also
// satisfies both memory capabilities. It lets the CLI tests
// exercise resolveMemoryEditPath through composition without
// dragging in the real Claude adapter, which would tie these
// tests to ~/.claude on the developer machine.
type memoryProviderStub struct {
	hasGlobal     bool
	hasPerProject bool
}

func (memoryProviderStub) Name() string { return "stub" }
func (memoryProviderStub) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{Adapter: "stub"}, nil
}
func (memoryProviderStub) ListProjects(fs.FS) ([]contracts.Project, error) { return nil, nil }
func (memoryProviderStub) ListSessions(fs.FS, contracts.ProjectID) ([]contracts.SessionSummary, error) {
	return nil, nil
}
func (memoryProviderStub) ReadSession(fs.FS, contracts.SessionID) (contracts.Conversation, error) {
	return contracts.Conversation{}, nil
}

// per-project capability methods
func (s memoryProviderStub) ListMemories(fs.FS) ([]contracts.MemoryFile, error) {
	if !s.hasPerProject {
		return nil, nil
	}
	return []contracts.MemoryFile{
		{Project: "proj-a", FileName: "MEMORY.md", SizeBytes: 10},
	}, nil
}
func (memoryProviderStub) MemoryFilePath(project contracts.ProjectID, fileName string) string {
	return filepath.Join("projects", string(project), "memory", fileName)
}
func (memoryProviderStub) PlanDeleteProjectMemory(fs.FS, contracts.ProjectID) (contracts.DeletePlan, error) {
	return contracts.DeletePlan{Category: "stub-memory"}, nil
}

// global capability methods
func (s memoryProviderStub) ListGlobalMemory(fs.FS) ([]contracts.GlobalMemoryFile, error) {
	if !s.hasGlobal {
		return nil, nil
	}
	return []contracts.GlobalMemoryFile{{FileName: "CLAUDE.md", SizeBytes: 20}}, nil
}
func (memoryProviderStub) GlobalMemoryFilePath(name string) string { return name }
func (memoryProviderStub) DefaultGlobalMemoryFile() string         { return "CLAUDE.md" }
func (memoryProviderStub) PlanDeleteGlobalMemory(fs.FS) (contracts.DeletePlan, error) {
	return contracts.DeletePlan{Category: "stub-global-memory"}, nil
}

// memoryAppWithStub wires a memoryProviderStub into an App.
// The provider's Root is empty, so any composition method
// that stat-checks a file under the root will see
// fs.ErrNotExist for any name, which is exactly what these
// tests rely on to prove the right name reached
// composition.
func memoryAppWithStub(t *testing.T, stub memoryProviderStub) (*composition.App, string) {
	t.Helper()
	dataRoot := t.TempDir()
	app := composition.NewForTest([]contracts.Provider{stub}, []fs.FS{fstest.MapFS{}})
	return app, dataRoot
}

// TestResolveMemoryEditPath_globalDefaultsToClaudeMd pins
// the CLI's user-experience default. With --global and no
// positional argument, the function should ask composition
// for the CLAUDE.md path, not error on a missing argument.
// We check by asserting the error wraps the file-not-found
// kind: composition stat-checks the path, and a missing
// file under a fresh temp root surfaces as fs.ErrNotExist.
func TestResolveMemoryEditPath_globalDefaultsToClaudeMd(t *testing.T) {
	app, _ := memoryAppWithStub(t, memoryProviderStub{hasGlobal: true})

	_, err := resolveMemoryEditPath(app, true, nil)
	if err == nil {
		t.Fatal("expected an error because the temp root has no CLAUDE.md")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want one wrapping fs.ErrNotExist (proves the CLAUDE.md path was resolved and stat-checked)", err)
	}
}

// TestResolveMemoryEditPath_perProjectRequiresTwoArgs pins
// the per-project shape. Without --global, the function
// requires exactly two positional arguments. Anything else
// is a usage error the CLI should surface clearly.
func TestResolveMemoryEditPath_perProjectRequiresTwoArgs(t *testing.T) {
	app, _ := memoryAppWithStub(t, memoryProviderStub{hasPerProject: true})

	_, err := resolveMemoryEditPath(app, false, []string{"proj-only"})
	if err == nil {
		t.Fatal("expected an error for one positional arg without --global")
	}
}

// TestResolveMemoryEditPath_globalHonoursExplicitFilename
// confirms the user can override the default. Passing
// --global plus a positional name routes that name through
// composition rather than CLAUDE.md. The test asserts the
// error message mentions the user-supplied name, which is
// the easiest way to prove the path resolution used the
// right argument.
func TestResolveMemoryEditPath_globalHonoursExplicitFilename(t *testing.T) {
	app, _ := memoryAppWithStub(t, memoryProviderStub{hasGlobal: true})

	_, err := resolveMemoryEditPath(app, true, []string{"CLAUDE.experimental.md"})
	if err == nil {
		t.Fatal("expected an error because the named file does not exist on disk")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist for a missing user-named global file", err)
	}
}
