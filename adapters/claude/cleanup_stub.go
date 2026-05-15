package claude

import (
	"errors"
	"io/fs"

	"github.com/danieljbfz/chronicle/contracts"
)

// ErrNotImplemented is the sentinel error the cleanup methods
// return today. The full cascade-aware version lands in a later
// release, once the trash subsystem is in place. Returning a
// sentinel rather than a hand-typed string lets callers test for
// it with errors.Is, and the rest of chronicle can treat
// "cleanup is not implemented yet" as a normal condition rather
// than as a crash.
var ErrNotImplemented = errors.New("claude: cleanup is not implemented yet")

// planDeleteStub is the stand-in for the real PlanDelete method
// that will land alongside the trash subsystem. We give it its own
// name so the Provider implementation in provider.go reads more
// clearly: "PlanDelete is currently the stub" rather than having
// an inline function literal that pretends to be real code.
func planDeleteStub(_ fs.FS, _ contracts.SessionID) (contracts.DeletePlan, error) {
	return contracts.DeletePlan{}, ErrNotImplemented
}

// planOrphanScanStub is the equivalent stand-in for the orphan-scan
// pass. Same reasoning: a named function in its own file makes the
// "not yet implemented" boundary explicit, so the future
// implementation can replace it without confusion.
func planOrphanScanStub(_ fs.FS) (contracts.DeletePlan, error) {
	return contracts.DeletePlan{}, ErrNotImplemented
}
