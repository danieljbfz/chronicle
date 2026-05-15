package contracts

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
type Provider interface {
	Name() string

	Detect(root fs.FS) (StorageVersion, error)

	ListProjects(root fs.FS) ([]Project, error)
	ListSessions(root fs.FS, project ProjectID) ([]SessionSummary, error)
	ReadSession(root fs.FS, id SessionID) (Conversation, error)

	PlanDelete(root fs.FS, id SessionID) (DeletePlan, error)
	PlanOrphanScan(root fs.FS) (DeletePlan, error)
}
