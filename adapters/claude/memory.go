package claude

import (
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

// MemoryEntry describes one memory file inside a project's
// per-project memory directory. Claude writes these markdown
// files at `projects/<encoded-cwd>/memory/<name>.md` and loads
// them at session start to remember things across sessions.
//
// MEMORY.md is the index file. It always loads first (the first
// 200 lines or 25 KB get pulled into context at every session
// start). Other files like architecture.md or debugging.md load
// on demand based on the conversation.
type MemoryEntry struct {
	// Project is the encoded-cwd identifier of the project the
	// memory belongs to. Same value every other Claude method
	// uses for project IDs, so callers can pass it around.
	Project contracts.ProjectID

	// FileName is the filename inside the memory directory,
	// like "MEMORY.md" or "architecture.md". The index file is
	// always called MEMORY.md.
	FileName string

	// SizeBytes is the file's on-disk size. Helps the user see
	// at a glance which files are heaviest.
	SizeBytes int64

	// ModifiedAt is the last-modified time. A memory file that
	// has not changed in months is a strong candidate for the
	// "this is stale, prune it" workflow.
	ModifiedAt time.Time
}

// ListMemoryFiles returns every memory file in every project
// that has a memory directory. Projects without a memory
// directory are simply absent from the result. The slice is
// sorted by project name and then by filename, so MEMORY.md
// appears first inside each project (alphabetical sort puts
// uppercase before lowercase).
func (p *Provider) ListMemoryFiles(root fs.FS) ([]MemoryEntry, error) {
	projects, err := fs.ReadDir(root, projectsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, newError("list memory files", projectsDir, err)
	}

	var entries []MemoryEntry
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
func readMemoryDir(root fs.FS, projectName string) ([]MemoryEntry, error) {
	dir := path.Join(projectsDir, projectName, memoryDir)
	entries, err := fs.ReadDir(root, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, newError("read memory dir", dir, err)
	}

	var out []MemoryEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, MemoryEntry{
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
func MemoryFilePath(project contracts.ProjectID, fileName string) string {
	return path.Join(projectsDir, string(project), memoryDir, fileName)
}

// PlanDeleteProjectMemory returns a DeletePlan that wipes every
// memory file in one project. The plan goes through chronicle's
// normal trash flow, so the user can restore the memory if they
// regret deleting it.
//
// We return a plan instead of doing the delete directly because
// the memory files are real user content. Routing through the
// dry-run-then-apply flow keeps the safety story consistent
// with `chronicle clean`.
func (p *Provider) PlanDeleteProjectMemory(root fs.FS, project contracts.ProjectID) (contracts.DeletePlan, error) {
	dir := path.Join(projectsDir, string(project), memoryDir)
	entries, err := fs.ReadDir(root, dir)
	if err != nil {
		return contracts.DeletePlan{}, newError("plan delete memory", dir, err)
	}

	plan := contracts.DeletePlan{
		Category: "claude-memory",
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		addItem(root, &plan, path.Join(dir, entry.Name()), "memory file")
	}
	return plan, nil
}
