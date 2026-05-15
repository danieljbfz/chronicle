package contracts

import "io/fs"

// stubProvider exists only so the compile-time interface checks
// below have a concrete type to point at. A real, fully-featured
// fake Provider for higher-layer tests lives next to those tests
// in the composition package.
type stubProvider struct{}

func (stubProvider) Name() string                                            { return "stub" }
func (stubProvider) Detect(fs.FS) (StorageVersion, error)                    { return StorageVersion{}, nil }
func (stubProvider) ListProjects(fs.FS) ([]Project, error)                   { return nil, nil }
func (stubProvider) ListSessions(fs.FS, ProjectID) ([]SessionSummary, error) { return nil, nil }
func (stubProvider) ReadSession(fs.FS, SessionID) (Conversation, error)      { return Conversation{}, nil }

// stubCleaner is a separate type that exercises the optional
// Cleaner interface. Splitting it from stubProvider is the point:
// the type system itself proves Provider and Cleaner are separable
// concerns. A real adapter can implement only Provider today and
// add Cleaner later when the trash subsystem lands.
type stubCleaner struct {
	stubProvider
}

func (stubCleaner) PlanDelete(fs.FS, SessionID) (DeletePlan, error) { return DeletePlan{}, nil }
func (stubCleaner) PlanOrphanScan(fs.FS) (DeletePlan, error)        { return DeletePlan{}, nil }

// The lines below are the standard Go way to ask the compiler
// "please confirm that this type satisfies that interface, and
// forget the resulting value." The blank identifier discards the
// value, and the type annotation forces the check. If a method
// ever drifts away from the interface declaration, the build
// fails right here, with an error message that names the missing
// or mismatched method. The same pattern appears at the bottom
// of every adapter implementation in the project.
var (
	_ Provider = stubProvider{}
	_ Provider = stubCleaner{}
	_ Cleaner  = stubCleaner{}
)
