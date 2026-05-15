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
//
// The global-memory equivalent is in GlobalMemoryStore. We
// keep the two surfaces distinct because not every adapter
// will support both: an adapter might expose per-project
// memory but have no concept of a user-global file, or the
// other way around.
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

// GlobalMemoryStore is the optional capability adapters
// implement when their tool keeps a user-wide memory file
// loaded into every session, regardless of project. Today
// Claude Code is the only adapter that qualifies, with
// ~/.claude/CLAUDE.md. The interface lives in contracts so
// any future tool with the same concept (a hypothetical
// Cursor or Antigravity adapter, for example) can plug in
// without touching composition or the CLI.
//
// The capability is intentionally separate from MemoryStore
// rather than a flag on it, so an adapter can implement
// either, both, or neither. A future tool that exposes only
// per-project memory without a user-global file will
// satisfy MemoryStore and not GlobalMemoryStore. The
// composition layer discovers the two capabilities
// independently with type assertions.
type GlobalMemoryStore interface {
	// ListGlobalMemory returns every user-global memory file
	// the provider knows about. The slice is empty (not
	// an error) when no global file is on disk, which is
	// the normal state for a fresh install.
	ListGlobalMemory(root fs.FS) ([]GlobalMemoryFile, error)

	// GlobalMemoryFilePath returns the path of one global
	// memory file inside the provider's root. Composition
	// combines this with the provider's absolute root to
	// find the file on disk for reading or editing.
	GlobalMemoryFilePath(fileName string) string

	// PlanDeleteGlobalMemory returns a deletion plan for
	// every user-global memory file the provider exposes.
	// The plan goes through chronicle's normal trash flow,
	// so a regretted clean can be undone.
	PlanDeleteGlobalMemory(root fs.FS) (DeletePlan, error)
}

// GlobalMemoryFile mirrors MemoryFile for user-global
// memory. The shape stays small on purpose. Today only
// Claude exposes one file (CLAUDE.md), but we keep the
// listing as a slice so a provider that grows multiple
// global files (CLAUDE.md plus an experimental sibling,
// say) does not need a contract change.
type GlobalMemoryFile struct {
	// FileName is the filename relative to the user-global
	// memory location, like "CLAUDE.md". The naming
	// convention belongs to the provider, not to chronicle.
	FileName string

	// SizeBytes is the file's on-disk size. The CLI shows
	// this so the user knows whether the file is the small
	// "few rules" kind or has grown into something worth
	// pruning.
	SizeBytes int64

	// ModifiedAt is the file's last-modified time. A
	// global file that has not changed in months is the
	// most common kind, because user-wide instructions
	// stabilise once they are written.
	ModifiedAt time.Time
}
