package copilot

import (
	"errors"
	"io/fs"

	"github.com/danieljbfz/chronicle/contracts"
)

// ErrNotImplemented is the sentinel error the cleanup methods
// return today. The full cascade-aware version lands in a later
// release, once the trash subsystem is in place.
//
// Returning a sentinel value lets callers test for it with
// errors.Is. If we returned a hand-typed string, callers would
// have to parse the message text to recognize the case, which is
// fragile. With the sentinel, the rest of chronicle treats
// "cleanup is not implemented yet" as a normal condition and
// keeps running.
var ErrNotImplemented = errors.New("copilot: cleanup is not implemented yet")

// planDeleteStub stands in for the real PlanDelete method. The
// real version has to know about chatEditingSessions/<id>/, the
// state.vscdb index entries, and (for Copilot CLI sessions) the
// metadata file under globalStorage. None of those exist yet.
func planDeleteStub(_ fs.FS, _ contracts.SessionID) (contracts.DeletePlan, error) {
	return contracts.DeletePlan{}, ErrNotImplemented
}

// planOrphanScanStub stands in for the orphan-scan pass. The real
// version walks chatEditingSessions/ looking for entries whose
// owning chat session no longer exists, and scans
// emptyWindowChatSessions/ for the same condition.
func planOrphanScanStub(_ fs.FS) (contracts.DeletePlan, error) {
	return contracts.DeletePlan{}, ErrNotImplemented
}
