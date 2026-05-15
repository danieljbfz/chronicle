package contracts

import "io/fs"

// stubProvider exists only so the compile-time interface check at the
// bottom of this file has a concrete type to point at. A real,
// fully-featured fake Provider for higher-layer tests lives next to
// those tests in the composition package.
type stubProvider struct{}

func (stubProvider) Name() string                                            { return "stub" }
func (stubProvider) Detect(fs.FS) (StorageVersion, error)                    { return StorageVersion{}, nil }
func (stubProvider) ListProjects(fs.FS) ([]Project, error)                   { return nil, nil }
func (stubProvider) ListSessions(fs.FS, ProjectID) ([]SessionSummary, error) { return nil, nil }
func (stubProvider) ReadSession(fs.FS, SessionID) (Conversation, error)      { return Conversation{}, nil }
func (stubProvider) PlanDelete(fs.FS, SessionID) (DeletePlan, error)         { return DeletePlan{}, nil }
func (stubProvider) PlanOrphanScan(fs.FS) (DeletePlan, error)                { return DeletePlan{}, nil }

// The line below is the standard Go way to ask the compiler "please
// confirm that this type satisfies that interface, and forget the
// resulting value." The blank identifier discards the value, and the
// type annotation forces the check. If a method ever drifts away from
// the interface declaration, the build fails right here, with an
// error message that names the missing or mismatched method. The same
// pattern appears at the bottom of every adapter implementation in
// the project.
var _ Provider = stubProvider{}
