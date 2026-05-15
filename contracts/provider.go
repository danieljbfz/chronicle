package contracts

// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. INTERFACES AS ARCHITECTURAL PORTS. The `Provider` interface below is
//    the central seam of chronicle's hexagonal architecture. The CLI, the
//    web frontend, and (later) the TUI all depend on this interface, not
//    on any concrete adapter. Adding a new tool (Cursor, Antigravity) is
//    a matter of writing a new type that satisfies Provider — the rest of
//    chronicle does not need to know which adapters exist.
//
// 2. MULTIPLE RETURN VALUES. Every method below returns `(result, error)`.
//    This is the canonical Go shape: the caller writes
//        result, err := provider.ReadSession(...)
//        if err != nil { return err }
//    Errors are values, not exceptions, and there is no try/except.
//    Python code typically raises `OSError` and lets the stack unwind
//    until something catches it; Go demands the caller decide what to
//    do with the error at the moment it appears. The pattern looks
//    repetitive but makes failure paths visible at every call site —
//    useful for a tool that reads other tools' shifting on-disk formats.
//
// 3. `io/fs` AND `fs.FS` — TESTABLE FILESYSTEMS. Methods take `root fs.FS`
//    rather than a path string. `fs.FS` is a tiny interface from the
//    standard library: anything with an `Open(name string) (fs.File, error)`
//    method satisfies it. Production code passes `os.DirFS("/home/u/.claude")`;
//    tests pass `fstest.MapFS{"projects/p/s.jsonl": &fstest.MapFile{...}}`.
//    The adapter has no idea whether it is reading the user's real disk
//    or a fixture in memory. This is the single most important pattern
//    for testable Go code, and we use it everywhere.

import "io/fs"

// Provider is the per-tool adapter contract. Composition passes each
// Provider an fs.FS rooted at the provider's data directory, so adapters
// never touch the real filesystem directly — that makes them trivially
// testable against fixture trees.
//
// Detect always returns a non-nil StorageVersion. Errors are reserved for
// two cases only: the path is unreachable, or no record in the storage is
// parseable as JSON at all. A file with valid JSON whose schema we do not
// recognize is Version = "unknown", not an error.
//
// Every Provider implementation must satisfy the resilience contract:
// detect storage version, parse tolerantly, advertise capabilities, and
// warn (not crash) on unknown shapes.
type Provider interface {
	Name() string

	Detect(root fs.FS) (StorageVersion, error)

	ListProjects(root fs.FS) ([]Project, error)
	ListSessions(root fs.FS, project ProjectID) ([]SessionSummary, error)
	ReadSession(root fs.FS, id SessionID) (Conversation, error)

	PlanDelete(root fs.FS, id SessionID) (DeletePlan, error)
	PlanOrphanScan(root fs.FS) (DeletePlan, error)
}
