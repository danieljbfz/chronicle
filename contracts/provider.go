package contracts

import "io/fs"

// Provider is the contract every per-tool adapter has to satisfy.
// The rest of chronicle calls these methods, never the adapters
// directly.
//
// Composition hands each Provider an fs.FS rooted at the
// provider's data directory, instead of a path string. That sounds
// like a small detail, but it is the trick that makes adapters
// easy to test. In production, composition passes
// os.DirFS("/home/user/.claude") and the adapter reads real files.
// In tests, the suite passes an in-memory fstest.MapFS filled
// with fixture content. The adapter cannot tell the difference,
// because both kinds of value satisfy the same fs.FS interface.
//
// Detect always returns a non-nil StorageVersion. There are only
// two cases where it returns an error instead. The first is when
// the path is unreachable (e.g., a permission denial or a missing root).
// The second is when no record in the file parses as JSON at all,
// which means we are not looking at the right kind of file. A
// file with valid JSON we do not recognize is not an error. We
// set Version to "unknown" and the rest of the system stays
// read-only.
//
// Every Provider has to follow four rules.
//
//  1. Detect the storage version from a fingerprint of the first
//     few records, so we can tell new versions apart from old ones.
//  2. Parse tolerantly. Record types and content kinds we do not
//     recognize become UnknownBlock values, never dropped silently.
//  3. Set the right Capabilities flags, so the user interface knows
//     which features to show without having to look at the version.
//  4. Write a structured warning report when an unrecognized
//     fingerprint shows up, so the user knows their data is being
//     read in read-only mode.
type Provider interface {
	Name() string

	Detect(root fs.FS) (StorageVersion, error)

	ListProjects(root fs.FS) ([]Project, error)
	ListSessions(root fs.FS, project ProjectID) ([]SessionSummary, error)
	ReadSession(root fs.FS, id SessionID) (Conversation, error)

	PlanDelete(root fs.FS, id SessionID) (DeletePlan, error)
	PlanOrphanScan(root fs.FS) (DeletePlan, error)
}
