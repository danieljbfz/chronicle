package contracts

import (
	"io/fs"
	"time"
)

// MemoryStore is the optional capability adapters implement
// when their tool keeps a per-project memory store. Today
// Claude Code is the only adapter that does, but the
// interface lives in contracts so any future tool with the
// same concept can plug in without touching composition or
// the CLI.
//
// Composition uses a type assertion to find which providers
// implement MemoryStore, the same way it finds Cleaner. This
// keeps the base Provider interface small and the memory
// surface visible in the type system.
//
// "Per-project memory" here means files the assistant tool
// writes per workspace and reads back at the start of every
// session in that workspace. The classic example is Claude's
// auto-memory at projects/<encoded-cwd>/memory/MEMORY.md and
// its topic files like architecture.md. They are user-facing
// content the user can edit or delete, but they are written
// by the tool and reloaded automatically.
type MemoryStore interface {
	// ListMemories returns every memory file across every
	// project the provider knows about. The slice is empty
	// when no project has memory enabled, which is a normal
	// state for a fresh install.
	ListMemories(root fs.FS) ([]MemoryFile, error)

	// MemoryFilePath returns the path of one memory file
	// inside the provider's root. Composition combines this
	// with the provider's absolute root to find the file on
	// disk for reading, editing, or deletion.
	MemoryFilePath(project ProjectID, fileName string) string

	// PlanDeleteProjectMemory returns a deletion plan for
	// every memory file in one project. The plan goes
	// through chronicle's normal trash flow, so the user can
	// restore the memory if they regret deleting it.
	PlanDeleteProjectMemory(root fs.FS, project ProjectID) (DeletePlan, error)
}

// MemoryFile describes one memory file inside a per-project
// memory store. The shape is intentionally small and uniform
// across providers, so the CLI can render every memory file
// the same way regardless of which tool wrote it.
type MemoryFile struct {
	// Project is the identifier of the project this memory
	// belongs to. Same value the provider uses for project
	// IDs everywhere else, so callers can pass it back into
	// other provider methods.
	Project ProjectID

	// FileName is the filename inside the memory directory,
	// like "MEMORY.md" or "architecture.md". The naming
	// convention belongs to the provider, not to chronicle.
	FileName string

	// SizeBytes is the file's on-disk size. Helps the user
	// see at a glance which memory files have grown large.
	SizeBytes int64

	// ModifiedAt is the file's last-modified time. A memory
	// file that has not changed in months is a strong
	// candidate for "this is stale, time to prune."
	ModifiedAt time.Time
}
