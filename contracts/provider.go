package contracts

import "io/fs"

// Provider is the per-tool adapter contract that the rest of chronicle
// uses. Composition hands each Provider an fs.FS rooted at the
// provider's data directory, instead of a path string, so that the
// adapter never touches the real filesystem directly. That indirection
// is what makes adapters trivially testable: production code passes
// os.DirFS("/home/user/.claude") and the tests pass an in-memory
// fstest.MapFS, and the adapter cannot tell the difference.
//
// Detect always returns a non-nil StorageVersion. We only return an
// error in the two cases where the adapter genuinely cannot proceed.
// The first is when the provided path is unreachable, for example
// because of a permission denial or a missing root directory. The
// second is when no record in the storage parses as JSON at all, which
// the resilience contract treats as a sign that we are pointed at a
// completely foreign file rather than at an unrecognized version of a
// known one. A file containing valid JSON whose schema we do not
// recognize is handled by returning Version equal to "unknown," not by
// returning an error.
//
// Every Provider implementation has to satisfy the four guarantees of
// the resilience contract. It detects the storage version through a
// fingerprint computed from the first few records. It parses
// tolerantly, so unknown record types and unknown content kinds become
// UnknownBlock entries instead of getting dropped. It tells the rest
// of chronicle what it understood through the Capabilities flags, so
// the user interface can branch on capability rather than on version.
// And it produces a structured warning report when it encounters an
// unrecognized fingerprint, so the user knows their data is being read
// in read-only mode.
type Provider interface {
	Name() string

	Detect(root fs.FS) (StorageVersion, error)

	ListProjects(root fs.FS) ([]Project, error)
	ListSessions(root fs.FS, project ProjectID) ([]SessionSummary, error)
	ReadSession(root fs.FS, id SessionID) (Conversation, error)

	PlanDelete(root fs.FS, id SessionID) (DeletePlan, error)
	PlanOrphanScan(root fs.FS) (DeletePlan, error)
}
