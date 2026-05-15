package claude

import (
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/danieljbfz/chronicle/contracts"
)

// Claude implements the contracts.MemoryStore optional
// capability. Per-project memory lives at
// `projects/<encoded-cwd>/memory/<name>.md`. Claude writes
// these markdown files automatically when auto-memory is on
// and loads them at session start to remember things across
// sessions. MEMORY.md is the index file. Other files like
// architecture.md or debugging.md load on demand based on
// the conversation.
//
// The MemoryStore methods below let chronicle's CLI list,
// show, edit, and delete these files without anyone outside
// the adapter needing to know how Claude stores them on disk.

// Categories and reasons used by the memory deletion plans.
// We keep them as named constants so the strings the trash
// listing displays are defined in one place. Adding a new
// kind of memory plan is one new constant here plus one use
// site below.
const (
	categoryProjectMemory = "claude-memory"
	categoryGlobalMemory  = "claude-global-memory"

	reasonProjectMemoryFile = "memory file"
	reasonGlobalMemoryFile  = "global memory file"
)

// ListMemories returns every memory file in every project
// that has a memory directory. Projects without one are
// simply absent from the result. The slice is sorted by
// project name and then by filename, so MEMORY.md appears
// first inside each project (uppercase sorts before lowercase
// in alphabetical order).
func (p *Provider) ListMemories(root fs.FS) ([]contracts.MemoryFile, error) {
	projects, err := fs.ReadDir(root, projectsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, newError("list memories", projectsDir, err)
	}

	var entries []contracts.MemoryFile
	for _, proj := range projects {
		if !proj.IsDir() {
			continue
		}
		more, err := readMemoryDir(root, proj.Name())
		if err != nil {
			return nil, err
		}
		entries = append(entries, more...)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Project != entries[j].Project {
			return entries[i].Project < entries[j].Project
		}
		return entries[i].FileName < entries[j].FileName
	})
	return entries, nil
}

// readMemoryDir walks one project's memory directory and
// returns the entries inside. The function returns an empty
// slice (not an error) when the project has no memory
// directory at all, because that just means the user has
// never enabled auto-memory for that project.
func readMemoryDir(root fs.FS, projectName string) ([]contracts.MemoryFile, error) {
	dir := path.Join(projectsDir, projectName, memoryDir)
	entries, err := fs.ReadDir(root, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, newError("read memory dir", dir, err)
	}

	var out []contracts.MemoryFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, contracts.MemoryFile{
			Project:    contracts.ProjectID(projectName),
			FileName:   entry.Name(),
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime(),
		})
	}
	return out, nil
}

// MemoryFilePath returns the relative path of one memory file
// inside the Claude root, suitable for joining with the
// provider's absolute root. Composition uses this to read,
// edit, or delete the file.
//
// The function does not check whether the file exists. The
// caller can stat the returned path if it cares about
// existence, but most callers either follow up with a read
// (which would fail anyway) or are about to write the file
// for the first time.
func (p *Provider) MemoryFilePath(project contracts.ProjectID, fileName string) string {
	return path.Join(projectsDir, string(project), memoryDir, fileName)
}

// PlanDeleteProjectMemory returns a DeletePlan that wipes
// every memory file in one project. The plan goes through
// chronicle's normal trash flow, so the user can restore the
// memory if they regret deleting it.
//
// We return a plan instead of doing the delete directly
// because the memory files are real user content. Routing
// through the dry-run-then-apply flow keeps the safety story
// consistent with `chronicle clean`.
func (p *Provider) PlanDeleteProjectMemory(root fs.FS, project contracts.ProjectID) (contracts.DeletePlan, error) {
	dir := path.Join(projectsDir, string(project), memoryDir)
	entries, err := fs.ReadDir(root, dir)
	if err != nil {
		return contracts.DeletePlan{}, newError("plan delete memory", dir, err)
	}

	plan := contracts.DeletePlan{
		Category: categoryProjectMemory,
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		addItem(root, &plan, path.Join(dir, entry.Name()), reasonProjectMemoryFile)
	}
	return plan, nil
}

// globalMemoryFile is the canonical user-global memory file
// Claude reads at the start of every session. Today this is
// the only known global memory file. If Claude adds a
// second one (a CLAUDE.experimental.md or similar), we
// extend the slice and the rest of the implementation keeps
// working unchanged.
const globalMemoryFile = "CLAUDE.md"

// ListGlobalMemory returns the user-global memory files the
// Claude install currently has on disk. The slice has zero
// or one entry today: zero when the user has never written
// CLAUDE.md, one when they have. We return a slice rather
// than a single value because the contract anticipates
// providers that grow more than one global file later, and
// the slice shape keeps the consumers uniform.
func (p *Provider) ListGlobalMemory(root fs.FS) ([]contracts.GlobalMemoryFile, error) {
	var out []contracts.GlobalMemoryFile
	for _, name := range globalMemoryFiles {
		info, err := fs.Stat(root, name)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, newError("list global memory", name, err)
		}
		if info.IsDir() {
			// A directory at this name is not a memory
			// file. Skip silently rather than fail, so a
			// future provider quirk does not break the
			// listing of the rest.
			continue
		}
		out = append(out, contracts.GlobalMemoryFile{
			FileName:   name,
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime(),
		})
	}
	return out, nil
}

// globalMemoryFiles is the set of filenames Claude treats as
// user-global memory at the top of the chronicle root. Today
// the set has one entry. We keep it as a slice constant so
// adding a second filename later is a one-line change with
// no other code to update.
var globalMemoryFiles = []string{globalMemoryFile}

// GlobalMemoryFilePath returns the path of one global memory
// file inside the Claude root. Composition joins it with the
// provider's absolute root to find the file on disk.
//
// We do not validate the filename against globalMemoryFiles
// here. The CLI is responsible for passing a valid name,
// and the caller's read or write will fail naturally with a
// "no such file" error if the name does not exist on disk.
func (p *Provider) GlobalMemoryFilePath(fileName string) string {
	return fileName
}

// DefaultGlobalMemoryFile is the filename the CLI assumes
// when the user asks for `chronicle memory show --global`
// (or edit, or clean) without naming a file. We return the
// canonical Claude name so a Claude user never has to type
// it. Other providers that implement this interface will
// return their own canonical name, which keeps the CLI
// flow uniform across providers without coupling the CLI
// to any one of them.
func (p *Provider) DefaultGlobalMemoryFile() string {
	return globalMemoryFile
}

// PlanDeleteGlobalMemory returns a DeletePlan that wipes
// every user-global memory file Claude knows about. The
// plan goes through chronicle's normal trash flow, so a
// regretted clean can be undone with `chronicle trash
// restore`.
//
// We deliberately scan the same set ListGlobalMemory uses,
// so the dry-run output matches what the user sees in the
// listing. A file that is not on disk produces no plan
// item, which is the correct behaviour: there is nothing to
// delete.
func (p *Provider) PlanDeleteGlobalMemory(root fs.FS) (contracts.DeletePlan, error) {
	plan := contracts.DeletePlan{
		Category: categoryGlobalMemory,
	}
	for _, name := range globalMemoryFiles {
		addItem(root, &plan, name, reasonGlobalMemoryFile)
	}
	return plan, nil
}

// Compile-time check: *Provider satisfies the optional
// contracts.MemoryStore capability. If we ever add a method
// to MemoryStore or change a signature, the build fails right
// here with an error that names the missing method. This is
// the protection that catches the exact kind of drift that
// produced the empty-memory-list bug during the first
// implementation pass.
var _ contracts.MemoryStore = (*Provider)(nil)

// Compile-time check: *Provider satisfies the optional
// contracts.GlobalMemoryStore capability. Same protection
// pattern as MemoryStore: a future contract change surfaces
// here at build time rather than as a runtime ok=false on
// the type assertion.
var _ contracts.GlobalMemoryStore = (*Provider)(nil)
