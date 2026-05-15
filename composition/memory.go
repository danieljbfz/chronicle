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
// MemoryFile rather than re-export it so the CLI has one
// stable presentation type and the provider stays free to
// evolve its own internal shape.
type MemoryListing struct {
	Provider   string
	ProjectID  contracts.ProjectID
	FileName   string
	SizeBytes  int64
	ModifiedAt string // RFC3339-style "2006-01-02 15:04" UTC
}

// ListMemories returns every memory file across every
// provider that implements the optional MemoryStore
// capability. Providers that do not implement it (Copilot
// today) simply contribute nothing. The function returns an
// empty slice (not an error) when no provider has memory
// data, which is the normal state for a fresh install.
//
// The CLI calls this for `chronicle memory list`. The same
// shape is ready for any future TUI or web view that wants
// to present memory files alongside sessions.
func (a *App) ListMemories() ([]MemoryListing, error) {
	var out []MemoryListing
	for _, p := range a.providers {
		store, ok := p.Provider.(contracts.MemoryStore)
		if !ok {
			continue
		}
		entries, err := store.ListMemories(p.FS)
		if err != nil {
			return nil, fmt.Errorf("composition.ListMemories: %s: %w", p.Provider.Name(), err)
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
	return out, nil
}

// ShowMemory writes the contents of one memory file to the
// given writer. The function is the back end for
// `chronicle memory show`, where the CLI pipes the output to
// stdout or to a pager.
//
// Callers pass the project ID and filename instead of a full
// path because that is what the user already saw in
// `chronicle memory list`. Asking them to type a path would
// mean they need to know the encoded-cwd encoding, which is
// exactly what chronicle exists to hide.
//
// The providerName argument disambiguates when more than one
// provider owns memory for the same project ID. Today
// Claude is the only provider with memory, so callers can
// pass the empty string to mean "find the first provider
// that has it." A future multi-provider memory world (if and
// when one arrives) would require the caller to be explicit.
func (a *App) ShowMemory(providerName string, project contracts.ProjectID, fileName string, w io.Writer) error {
	entry, store, err := a.findMemoryStore(providerName)
	if err != nil {
		return err
	}
	relative := store.MemoryFilePath(project, fileName)
	full := filepath.Join(entry.Root, relative)

	data, err := os.ReadFile(full)
	if err != nil {
		return fmt.Errorf("composition.ShowMemory: %w", err)
	}
	_, err = w.Write(data)
	return err
}

// EditMemoryPath returns the absolute filesystem path of one
// memory file. The CLI uses this to spawn `$EDITOR` on the
// file. We expose the path instead of doing the editor spawn
// ourselves because editor-spawning belongs in the CLI
// layer, not in the application core. Composition stays
// focused on read and write, the CLI handles process
// management.
//
// The function checks that the file exists before returning
// the path. Opening a missing file in `$EDITOR` would create
// an empty new file on save, which is not what the user
// asked for. They want to edit something the assistant
// wrote, not author a new memory.
func (a *App) EditMemoryPath(providerName string, project contracts.ProjectID, fileName string) (string, error) {
	entry, store, err := a.findMemoryStore(providerName)
	if err != nil {
		return "", err
	}
	relative := store.MemoryFilePath(project, fileName)
	full := filepath.Join(entry.Root, relative)
	if _, err := os.Stat(full); err != nil {
		return "", fmt.Errorf("composition.EditMemoryPath: %w", err)
	}
	return full, nil
}

// CleanProjectMemory builds a plan to move every memory file
// in one project into the trash. The function is the back
// end for `chronicle memory clean <project>`. Like the other
// clean commands, it returns a PlannedDeletion the caller
// can either render (dry-run) or execute. Going through the
// same flow means the user gets the same safety story they
// already get from `chronicle clean`: trash first, restore
// if regretted, nothing is permanently lost until the trash
// is emptied.
func (a *App) CleanProjectMemory(providerName string, project contracts.ProjectID) (PlannedDeletion, error) {
	entry, store, err := a.findMemoryStore(providerName)
	if err != nil {
		return PlannedDeletion{}, err
	}
	plan, err := store.PlanDeleteProjectMemory(entry.FS, project)
	if err != nil {
		return PlannedDeletion{}, fmt.Errorf("composition.CleanProjectMemory: %w", err)
	}
	return PlannedDeletion{provider: entry, Plan: plan}, nil
}

// findMemoryStore returns the providerEntry and MemoryStore
// for the named provider. When providerName is empty, the
// function returns the first registered provider that
// implements MemoryStore. The split between empty-name and
// named lookup mirrors the rest of the CLI: most users have
// just one memory-capable provider and want to skip the
// disambiguation, but power users with several can name the
// one they want.
func (a *App) findMemoryStore(providerName string) (*providerEntry, contracts.MemoryStore, error) {
	for _, p := range a.providers {
		if providerName != "" && p.Provider.Name() != providerName {
			continue
		}
		store, ok := p.Provider.(contracts.MemoryStore)
		if !ok {
			continue
		}
		return p, store, nil
	}
	if providerName != "" {
		return nil, nil, fmt.Errorf("provider %q does not support memory operations", providerName)
	}
	return nil, nil, errors.New("no registered provider supports memory operations")
}
