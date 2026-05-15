package contracts

// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. THE COMPILE-TIME INTERFACE CHECK. The bottom-of-file line
//        var _ Provider = stubProvider{}
//    is a Go idiom that says: "the compiler must prove that
//    `stubProvider{}` satisfies the `Provider` interface, but I do not
//    need to keep the resulting value." The blank identifier `_` discards
//    it; the type annotation `Provider` is what triggers the check. If
//    `stubProvider` ever drifts away from `Provider` (a missing method,
//    a signature change), the build fails at this line with a clear
//    error message. Zero runtime cost — it is a static guarantee only.
//    We use this pattern at the bottom of every adapter implementation.
//
// 2. TEST FILES LIVE NEXT TO SOURCE. The file `provider_test.go` sits in
//    the same package as `provider.go`. The `_test.go` suffix is what
//    tells `go test` this is a test file. Test files are excluded from
//    the regular build, so test-only helpers do not bloat the binary.
//
// 3. UNEXPORTED HELPER TYPES. `stubProvider` starts with a lowercase
//    letter, so it is private to the `contracts` package. The compile
//    check above is the only reference to it.

import "io/fs"

// stubProvider exists only to prove the Provider interface compiles. A
// real fake for higher-layer tests lives next to those tests (see
// composition/browse_test.go).
type stubProvider struct{}

func (stubProvider) Name() string                                            { return "stub" }
func (stubProvider) Detect(fs.FS) (StorageVersion, error)                    { return StorageVersion{}, nil }
func (stubProvider) ListProjects(fs.FS) ([]Project, error)                   { return nil, nil }
func (stubProvider) ListSessions(fs.FS, ProjectID) ([]SessionSummary, error) { return nil, nil }
func (stubProvider) ReadSession(fs.FS, SessionID) (Conversation, error)      { return Conversation{}, nil }
func (stubProvider) PlanDelete(fs.FS, SessionID) (DeletePlan, error)         { return DeletePlan{}, nil }
func (stubProvider) PlanOrphanScan(fs.FS) (DeletePlan, error)                { return DeletePlan{}, nil }

// Compile-time check: stubProvider satisfies contracts.Provider. If a
// method ever disappears or its signature changes, this line stops
// compiling with a clear error. See concept #1 above.
var _ Provider = stubProvider{}
