package contracts

import "io/fs"

// GlobalConfig is the optional capability adapters
// implement when their tool keeps a user-wide configuration
// file with a per-project subsection. Today Claude Code is
// the only adapter that qualifies, with ~/.claude.json
// holding a `projects` map keyed by absolute working
// directory. The interface lives in contracts so any future
// tool with the same shape (a hypothetical Cursor or
// Antigravity adapter that records per-project state in a
// user-wide JSON file) can plug in without composition or
// the CLI learning the new tool's specifics.
//
// Composition discovers GlobalConfig the same way it finds
// the other capability interfaces, with a type assertion at
// the call site. Keeping the surface optional means a
// provider that does not have a global-config file (Copilot
// today) does not have to ship stub methods.
//
// The interface is deliberately narrow. It exposes only the
// per-project entry inspection and removal flow that
// chronicle's `clean config-projects` command needs.
// Editing individual fields inside a project entry is out
// of scope: chronicle removes whole entries when their
// project directory has gone, and leaves everything else to
// the upstream tool's own config commands.
type GlobalConfig interface {
	// ListConfigProjectEntries returns one entry per
	// project recorded in the global config file. The
	// caller can inspect the Exists flag to decide which
	// entries are stale candidates for removal.
	//
	// The slice is empty (not an error) when the global
	// config file is missing or has no projects subsection,
	// which is the normal state for a fresh install.
	ListConfigProjectEntries(root fs.FS) ([]ConfigProjectEntry, error)

	// RemoveConfigProjectEntries rewrites the global config
	// file with the named keys removed from the projects
	// subsection. The implementation is responsible for the
	// safety semantics: backup the original file before
	// writing, write atomically (temp file plus rename),
	// and roll back if any step fails.
	//
	// keys is the slice of project keys (the absolute
	// working-directory strings) to remove. Keys that are
	// not present in the projects subsection are skipped
	// silently, so a stale plan does not block a real
	// removal.
	//
	// The function returns the path of the backup file the
	// implementation wrote before the edit. The caller can
	// show that path to the user, so the user knows where to
	// restore from if the removal turns out to have been a
	// mistake.
	RemoveConfigProjectEntries(root fs.FS, keys []string) (backupPath string, err error)
}

// ConfigProjectEntry describes one entry in a global config
// file's per-project subsection. The shape is intentionally
// small. Chronicle does not surface the rich per-project
// state (MCP server configs, last-session metrics, trust
// dialog flags) because the cleanup story is "remove the
// whole entry when its directory is gone," not "edit a
// field inside the entry."
type ConfigProjectEntry struct {
	// Key is the project identifier the global config uses,
	// like "/Users/djbf/Desktop/work/claude-history" for
	// Claude. The same value is what the caller passes back
	// to RemoveConfigProjectEntries.
	Key string

	// Exists reports whether the directory the Key names
	// currently resolves on disk. A false value is the
	// signal chronicle uses to flag the entry as stale.
	// True means the directory is still there, so the entry
	// is presumed valid.
	Exists bool

	// SizeBytes is the on-disk size of the entry's JSON
	// body, useful for the dry-run output so the user can
	// see how much config bloat one stale entry represents.
	SizeBytes int64
}
