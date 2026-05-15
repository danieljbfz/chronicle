package copilot

import (
	"errors"
	"io/fs"
	"path"
	"strings"

	"github.com/danieljbfz/chronicle/contracts"
)

// chatEditingSessionsDir is the directory under each VS Code
// workspace that holds per-session edit snapshots. Removing a
// session must take this whole subtree with it, or we leave
// orphans on disk that the user has no idea exist.
const chatEditingSessionsDir = "chatEditingSessions"

// categoryCopilotSession is the label every per-session deletion
// plan carries, mirroring the Claude adapter's shape. The trash
// listing uses it to group entries that came from the same kind
// of operation.
const categoryCopilotSession = "copilot-session"

// PlanDelete returns the cascade plan for deleting one Copilot
// chat session. The plan includes the session's .jsonl file
// and the matching chatEditingSessions/<id>/ directory of edit
// snapshots, if one exists.
//
// The session file lives in one of two places. Most chats sit
// under workspaceStorage/<hash>/chatSessions/ for the
// workspace they belong to. Chats started in folder-less VS
// Code windows live under globalStorage/emptyWindowChatSessions
// instead. Empty-window chats do not have edit snapshots, so
// we skip that lookup for them.
//
// Any sibling that does not exist on disk is dropped silently
// from the plan, so the dry-run output stays focused on the
// real artifacts.
//
// PlanDelete returns a wrapped fs.ErrNotExist when the session
// is not found in either location. Callers should test for
// that with errors.Is and treat it as "no such session"
// instead of as a hard failure.
func (p *Provider) PlanDelete(root fs.FS, id contracts.SessionID) (contracts.DeletePlan, error) {
	sessionFile, workspace, ok := locateInWorkspaces(root, id)
	switch {
	case ok:
		return planDeleteWorkspaceSession(root, id, sessionFile, workspace), nil
	}

	if file, ok := locateInEmptyWindows(root, id); ok {
		plan := contracts.DeletePlan{
			SessionID: id,
			Category:  categoryCopilotSession,
		}
		addItem(root, &plan, file, "session file (folder-less window)")
		return plan, nil
	}

	return contracts.DeletePlan{}, newError("plan delete", string(id), fs.ErrNotExist)
}

// planDeleteWorkspaceSession builds the cascade plan for a
// session that lives inside a workspace, including its edit
// snapshots when one exists. The function is split out from
// PlanDelete so each branch (workspace session vs empty-window
// session) stays short and reads top-to-bottom without nested
// conditionals.
func planDeleteWorkspaceSession(root fs.FS, id contracts.SessionID, sessionFile string, workspace contracts.ProjectID) contracts.DeletePlan {
	plan := contracts.DeletePlan{
		SessionID: id,
		Category:  categoryCopilotSession,
	}
	addItem(root, &plan, sessionFile, "session file")
	editingDir := path.Join(workspaceStorageDir, string(workspace), chatEditingSessionsDir, string(id))
	addItem(root, &plan, editingDir, "edit snapshots")
	return plan
}

// PlanOrphanScan walks every workspace looking for
// chatEditingSessions/<id>/ directories whose owning chat
// session is no longer present in chatSessions/. Each orphan
// becomes one item in the returned plan.
//
// We do not scan globalStorage during the orphan pass because
// empty-window chats do not have edit snapshots. There is
// simply nothing to look for there.
func (p *Provider) PlanOrphanScan(root fs.FS) (contracts.DeletePlan, error) {
	workspaces, err := fs.ReadDir(root, workspaceStorageDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return contracts.DeletePlan{Category: "copilot-orphans"}, nil
		}
		return contracts.DeletePlan{}, newError("plan orphan scan", workspaceStorageDir, err)
	}

	plan := contracts.DeletePlan{Category: "copilot-orphans"}
	for _, ws := range workspaces {
		if !ws.IsDir() {
			continue
		}
		if err := scanWorkspaceOrphans(root, ws.Name(), &plan); err != nil {
			return contracts.DeletePlan{}, newError("plan orphan scan", ws.Name(), err)
		}
	}

	// In addition to the per-workspace orphans above, scan for
	// the floating-junk files that have nothing to do with a
	// specific workspace. These live under globalStorage and
	// have their own heuristics in orphans.go.
	scanFloatingOrphans(root, &plan)
	return plan, nil
}

// scanWorkspaceOrphans checks one workspace's
// chatEditingSessions/ directory for entries whose owning chat
// session is no longer present. Live sessions are left alone,
// and the orphans get added to the plan with a clear Reason
// so the user can tell what they are looking at.
func scanWorkspaceOrphans(root fs.FS, workspace string, plan *contracts.DeletePlan) error {
	known := knownSessionsInWorkspace(root, workspace)

	editingRoot := path.Join(workspaceStorageDir, workspace, chatEditingSessionsDir)
	entries, err := fs.ReadDir(root, editingRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if known[entry.Name()] {
			continue
		}
		addItem(root, plan, path.Join(editingRoot, entry.Name()), "orphaned edit snapshots")
	}
	return nil
}

// knownSessionsInWorkspace returns the set of session UUIDs
// that still own a .jsonl file in the workspace's
// chatSessions/ directory. The orphan scan compares each
// entry under chatEditingSessions/ against this set, and any
// entry whose UUID is not in the set is an orphan.
func knownSessionsInWorkspace(root fs.FS, workspace string) map[string]bool {
	known := map[string]bool{}
	dir := path.Join(workspaceStorageDir, workspace, chatSessionsDir)
	entries, err := fs.ReadDir(root, dir)
	if err != nil {
		return known
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		known[strings.TrimSuffix(entry.Name(), ".jsonl")] = true
	}
	return known
}

// addItem appends a path to the plan, but only when the path
// actually exists on disk. Missing paths are dropped silently
// so the dry-run output stays focused on real artifacts. For a
// directory, the size is computed by walking the tree and
// summing every file inside. The walk is cheap compared to
// actually moving the data, so we pay the cost up front to
// give the user an accurate total.
func addItem(root fs.FS, plan *contracts.DeletePlan, relativePath, reason string) {
	info, err := fs.Stat(root, relativePath)
	if err != nil {
		return
	}
	size := info.Size()
	if info.IsDir() {
		size = directorySize(root, relativePath)
	}
	plan.Items = append(plan.Items, contracts.DeleteItem{
		Path:      relativePath,
		Reason:    reason,
		SizeBytes: size,
	})
	plan.SizeBytes += size
}

// directorySize sums the file sizes inside a directory tree.
// Errors during the walk are swallowed, so a directory we can
// only partially read produces an underestimate of the size
// instead of an error. The cost of a slightly-low number is
// minor compared to refusing to plan a cleanup at all.
func directorySize(root fs.FS, dir string) int64 {
	var total int64
	_ = fs.WalkDir(root, dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // walk continues past per-entry errors
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// Compile-time check: *Provider satisfies contracts.Cleaner now
// that the cleanup methods are in place.
var _ contracts.Cleaner = (*Provider)(nil)
