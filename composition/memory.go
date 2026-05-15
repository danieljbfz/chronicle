package composition

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/danieljbfz/chronicle/contracts"
)

// MemoryListing pairs one memory file with the provider info
// the CLI needs to display it. We wrap the contract's
// MemoryFile rather than re-exporting it so the CLI has one
// stable presentation type and the provider stays free to
// evolve its own internal shape.
type MemoryListing struct {
	Provider   string
	ProjectID  contracts.ProjectID
	FileName   string
	SizeBytes  int64
	ModifiedAt string // human-readable, formatted as "2006-01-02 15:04" UTC
}

// ListMemories returns every memory file across every
// provider, covering both the per-project and the
// user-global scopes. Providers contribute through whichever
// of the two optional capabilities they implement (or both,
// or neither). The function returns an empty slice (not an
// error) when no provider has memory data of either kind,
// which is the normal state for a fresh install.
//
// User-global entries appear with an empty MemoryListing
// ProjectID so the renderer can distinguish them from
// per-project rows. The CLI uses this for `chronicle memory
// list`. The same shape is ready for any future TUI or web
// view that wants to present memory files alongside
// sessions.
func (a *App) ListMemories() ([]MemoryListing, error) {
	var out []MemoryListing
	for _, p := range a.providers {
		if store, ok := p.Provider.(contracts.MemoryStore); ok {
			entries, err := store.ListMemories(p.FS)
			if err != nil {
				return nil, fmt.Errorf("memory list: %s: %w", p.Provider.Name(), err)
			}
			for _, entry := range entries {
				out = append(out, MemoryListing{
					Provider:   p.Provider.Name(),
					ProjectID:  entry.Project,
					FileName:   entry.FileName,
					SizeBytes:  entry.SizeBytes,
					ModifiedAt: entry.ModifiedAt.UTC().Format("2006-01-02 15:04"),
				})
			}
		}
		if globalStore, ok := p.Provider.(contracts.GlobalMemoryStore); ok {
			entries, err := globalStore.ListGlobalMemory(p.FS)
			if err != nil {
				return nil, fmt.Errorf("memory list global: %s: %w", p.Provider.Name(), err)
			}
			for _, entry := range entries {
				out = append(out, MemoryListing{
					Provider:   p.Provider.Name(),
					ProjectID:  "",
					FileName:   entry.FileName,
					SizeBytes:  entry.SizeBytes,
					ModifiedAt: entry.ModifiedAt.UTC().Format("2006-01-02 15:04"),
				})
			}
		}
	}
	return out, nil
}

// ShowMemory writes the contents of one memory file to the
// given writer. The function is the back end for
// `chronicle memory show`, where the CLI pipes the output to
// stdout or to a pager.
//
// Callers pass the project ID and filename instead of a full
// path because those are the values the user already saw in
// `chronicle memory list`. Asking them to type a path would
// mean they need to know the encoded-cwd encoding, which is
// exactly what chronicle exists to hide.
func (a *App) ShowMemory(project contracts.ProjectID, fileName string, w io.Writer) error {
	entry, store, err := a.findMemoryStore()
	if err != nil {
		return err
	}
	relative := store.MemoryFilePath(project, fileName)
	full := filepath.Join(entry.Root, relative)

	data, err := os.ReadFile(full)
	if err != nil {
		return fmt.Errorf("memory show: %w", err)
	}
	_, err = w.Write(data)
	return err
}

// EditMemoryPath returns the absolute filesystem path of one
// memory file. The CLI uses this to spawn `$EDITOR` on the
// file. We expose the path instead of doing the editor spawn
// ourselves because editor-spawning belongs in the CLI
// layer, not in the application core. Composition stays
// focused on read and write, and the CLI handles process
// management.
//
// The function checks that the file exists before returning
// the path. Opening a missing file in `$EDITOR` would create
// an empty new file on save, which is not what the user
// asked for. They want to edit something the assistant
// wrote, not author a new memory.
func (a *App) EditMemoryPath(project contracts.ProjectID, fileName string) (string, error) {
	entry, store, err := a.findMemoryStore()
	if err != nil {
		return "", err
	}
	relative := store.MemoryFilePath(project, fileName)
	full := filepath.Join(entry.Root, relative)
	if _, err := os.Stat(full); err != nil {
		return "", fmt.Errorf("memory edit: %w", err)
	}
	return full, nil
}

// CleanProjectMemory builds a plan to move every memory file
// in one project into the trash. The function is the back
// end for `chronicle memory clean <project>`. Like the other
// clean commands, it returns a PlannedDeletion the caller
// can either render (dry-run) or execute. Routing through
// the same flow means the user gets the same safety story
// they already get from `chronicle clean`: trash first,
// restore if regretted, nothing is permanently lost until
// the trash is emptied.
func (a *App) CleanProjectMemory(project contracts.ProjectID) (PlannedDeletion, error) {
	entry, store, err := a.findMemoryStore()
	if err != nil {
		return PlannedDeletion{}, err
	}
	plan, err := store.PlanDeleteProjectMemory(entry.FS, project)
	if err != nil {
		return PlannedDeletion{}, fmt.Errorf("memory clean: %w", err)
	}
	return PlannedDeletion{provider: entry, Plan: plan}, nil
}

// findMemoryStore returns the first registered provider that
// implements contracts.MemoryStore, along with its
// providerEntry. The function exists because every memory
// operation in this file needs the same lookup, and sharing
// the implementation keeps the per-method bodies focused on
// their own work.
//
// Today the lookup just picks the first match because Claude
// is the only adapter with memory. The day a second adapter
// ships memory, we will need to disambiguate, and the right
// shape for that will be obvious once we have two concrete
// callers to design against. Until then we keep the lookup
// simple.
func (a *App) findMemoryStore() (*providerEntry, contracts.MemoryStore, error) {
	for _, p := range a.providers {
		store, ok := p.Provider.(contracts.MemoryStore)
		if !ok {
			continue
		}
		return p, store, nil
	}
	return nil, nil, errors.New("no registered provider supports memory operations")
}

// ShowGlobalMemory writes one user-global memory file to the
// given writer. The function is the back end for `chronicle
// memory show --global`. It mirrors ShowMemory, but routes
// through the GlobalMemoryStore capability instead of the
// per-project MemoryStore.
//
// fileName is the global memory filename, like "CLAUDE.md".
// The caller (today the CLI) defaults this when the user
// passes --global without a filename, because the canonical
// case is the one global file Claude reads at every session
// start.
func (a *App) ShowGlobalMemory(fileName string, w io.Writer) error {
	entry, store, err := a.findGlobalMemoryStore()
	if err != nil {
		return err
	}
	relative := store.GlobalMemoryFilePath(fileName)
	full := filepath.Join(entry.Root, relative)

	data, err := os.ReadFile(full)
	if err != nil {
		return fmt.Errorf("memory show global: %w", err)
	}
	_, err = w.Write(data)
	return err
}

// EditGlobalMemoryPath returns the absolute filesystem path
// of one user-global memory file. The CLI uses this to spawn
// $EDITOR on the file. We expose the path instead of doing
// the editor spawn ourselves because editor-spawning belongs
// in the CLI layer, not in the application core. Composition
// stays focused on read and write, the CLI handles process
// management.
//
// The function checks the file exists before returning the
// path. Opening a missing file in $EDITOR would create an
// empty new file on save, which is not what the user asked
// for. A user-global memory file that does not exist yet is
// a different operation (a "create" rather than an "edit"),
// and chronicle deliberately stays out of that flow because
// the user has clear means to author the file themselves.
func (a *App) EditGlobalMemoryPath(fileName string) (string, error) {
	entry, store, err := a.findGlobalMemoryStore()
	if err != nil {
		return "", err
	}
	relative := store.GlobalMemoryFilePath(fileName)
	full := filepath.Join(entry.Root, relative)
	if _, err := os.Stat(full); err != nil {
		return "", fmt.Errorf("memory edit global: %w", err)
	}
	return full, nil
}

// CleanGlobalMemory builds a plan to move every user-global
// memory file into the trash. The function is the back end
// for `chronicle memory clean --global`. Like the per-project
// equivalent, it returns a PlannedDeletion the caller can
// either render (dry-run) or execute. Routing through the
// same flow means the user gets the same safety story they
// already get from `chronicle clean`: trash first, restore
// if regretted, nothing is permanently lost until the trash
// is emptied.
func (a *App) CleanGlobalMemory() (PlannedDeletion, error) {
	entry, store, err := a.findGlobalMemoryStore()
	if err != nil {
		return PlannedDeletion{}, err
	}
	plan, err := store.PlanDeleteGlobalMemory(entry.FS)
	if err != nil {
		return PlannedDeletion{}, fmt.Errorf("memory clean global: %w", err)
	}
	return PlannedDeletion{provider: entry, Plan: plan}, nil
}

// findGlobalMemoryStore mirrors findMemoryStore for the
// global capability. We split the two lookups because a
// future provider might support per-project memory without
// a user-global file, or the other way around, and the
// caller should get a clear error specific to which kind it
// asked for.
func (a *App) findGlobalMemoryStore() (*providerEntry, contracts.GlobalMemoryStore, error) {
	for _, p := range a.providers {
		store, ok := p.Provider.(contracts.GlobalMemoryStore)
		if !ok {
			continue
		}
		return p, store, nil
	}
	return nil, nil, errors.New("no registered provider supports user-global memory")
}

// DefaultGlobalMemoryFile returns the canonical filename
// the active provider uses for its user-global memory. The
// CLI calls this when the user runs `chronicle memory show
// --global` without naming a file, so each provider's own
// convention drives the default rather than a hardcoded
// CLI-side value.
//
// Returns an error when no provider implements
// GlobalMemoryStore. That is the same error the show, edit,
// and clean methods would surface, so the CLI can present
// it once consistently.
func (a *App) DefaultGlobalMemoryFile() (string, error) {
	_, store, err := a.findGlobalMemoryStore()
	if err != nil {
		return "", err
	}
	return store.DefaultGlobalMemoryFile(), nil
}
