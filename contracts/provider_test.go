package contracts

import "io/fs"

// stubProvider exists only to prove the Provider interface compiles. A real
// stub for tests in higher layers lives next to those tests.
type stubProvider struct{}

func (stubProvider) Name() string                                            { return "stub" }
func (stubProvider) Detect(fs.FS) (StorageVersion, error)                    { return StorageVersion{}, nil }
func (stubProvider) ListProjects(fs.FS) ([]Project, error)                   { return nil, nil }
func (stubProvider) ListSessions(fs.FS, ProjectID) ([]SessionSummary, error) { return nil, nil }
func (stubProvider) ReadSession(fs.FS, SessionID) (Conversation, error)      { return Conversation{}, nil }
func (stubProvider) PlanDelete(fs.FS, SessionID) (DeletePlan, error)         { return DeletePlan{}, nil }
func (stubProvider) PlanOrphanScan(fs.FS) (DeletePlan, error)                { return DeletePlan{}, nil }

var _ Provider = stubProvider{}
