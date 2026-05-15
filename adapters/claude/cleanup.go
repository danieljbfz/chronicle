package claude

import (
	"errors"
	"io/fs"
	"path"
	"strings"

	"github.com/danieljbfz/chronicle/contracts"
)

// The cascade-delete map for Claude Code. Removing one session
// from chronicle has to take that session's sibling artifacts
// with it, or we leave orphans on disk that the user has no
// idea exist. The research notes on Claude's storage layout
// describe every sibling, and we codify them here so the
// adapter has one source of truth for "what does it actually
// mean to delete a Claude session?"
//
// Each constant below is the name of a subdirectory under the
// Claude root that holds per-session data. Inside each one,
// every entry is named after a session UUID. So deleting
// session <id> means also deleting `<sibling>/<id>`, whether
// that turns out to be a file or a directory.
const (
	fileHistoryDir = "file-history"
	tasksDir       = "tasks"
	sessionEnvDir  = "session-env"
	sessionsDir    = "sessions"
)

// categoryClaudeSession is the label every per-session deletion
// plan carries. The trash listing uses it to group entries that
// came from the same kind of operation.
const categoryClaudeSession = "claude-session"

// PlanDelete returns the cascade plan for deleting one Claude
// session. The plan lists every artifact that session owns on
// disk:
//
//   - The .jsonl session file under projects/<cwd>/<id>.jsonl
//   - The file-history/<id>/ directory of versioned file backups
//   - The tasks/<id>/ directory of task state
//   - The session-env/<id> file of captured environment
//   - The sessions/<id>.json metadata file
//
// Any sibling that does not exist on disk is dropped silently
// from the plan. We never include a path the user would not
// see if they ran `ls` themselves, which keeps the dry-run
// output focused on the real artifacts.
//
// PlanDelete returns a wrapped fs.ErrNotExist when the session
// itself is missing. Callers should test for that with
// errors.Is and treat it as "no such session" instead of as a
// hard failure.
func (p *Provider) PlanDelete(root fs.FS, id contracts.SessionID) (contracts.DeletePlan, error) {
	sessionFile, err := locateSessionFile(root, id)
	if err != nil {
		return contracts.DeletePlan{}, newError("plan delete", string(id), err)
	}

	plan := contracts.DeletePlan{
		SessionID: id,
		Category:  categoryClaudeSession,
	}

	// The session file itself is always the first item. We use
	// addItem (below) to skip silently when something is
	// missing, which lets us list every potential sibling and
	// only keep the ones that really exist.
	addItem(root, &plan, sessionFile, "session file")

	// Each sibling lives under the Claude root and is keyed by
	// the session UUID. Some are files, some are directories.
	// addItem handles both shapes.
	addItem(root, &plan, path.Join(fileHistoryDir, string(id)), "file history")
	addItem(root, &plan, path.Join(tasksDir, string(id)), "task state")
	addItem(root, &plan, path.Join(sessionEnvDir, string(id)), "captured environment")
	addItem(root, &plan, path.Join(sessionsDir, string(id)+".json"), "session metadata")

	return plan, nil
}

// PlanOrphanScan walks the Claude sibling directories looking
// for entries whose owning session no longer exists under
// projects/. Each orphan becomes one item in the returned plan,
// labeled with a Reason that names the kind of orphan it is.
//
// The version-one chronicle does not expose this method
// through the CLI yet. Once `chronicle clean orphans` lands,
// it will call PlanOrphanScan and route the returned plan
// through composition.Trash the same way per-session deletes
// go through it today.
func (p *Provider) PlanOrphanScan(root fs.FS) (contracts.DeletePlan, error) {
	knownSessions, err := collectKnownSessionIDs(root)
	if err != nil {
		return contracts.DeletePlan{}, newError("plan orphan scan", "", err)
	}

	plan := contracts.DeletePlan{
		Category: "claude-orphans",
	}
	for _, sibling := range orphanSiblings {
		entries, err := fs.ReadDir(root, sibling.dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return contracts.DeletePlan{}, newError("plan orphan scan", sibling.dir, err)
		}
		for _, entry := range entries {
			id := strings.TrimSuffix(entry.Name(), sibling.suffix)
			if knownSessions[id] {
				continue
			}
			addItem(root, &plan, path.Join(sibling.dir, entry.Name()), sibling.reason)
		}
	}
	return plan, nil
}

// orphanSibling describes one sibling directory the orphan scan
// walks. The suffix lets us strip ".json" off entries in the
// sessions/ directory so we get back a comparable session id.
type orphanSibling struct {
	dir    string
	suffix string
	reason string
}

// orphanSiblings is the static list of directories we walk during
// an orphan scan. Adding a new sibling to the cascade map is one
// new entry here.
var orphanSiblings = []orphanSibling{
	{dir: fileHistoryDir, reason: "orphaned file history"},
	{dir: tasksDir, reason: "orphaned task state"},
	{dir: sessionEnvDir, reason: "orphaned environment capture"},
	{dir: sessionsDir, suffix: ".json", reason: "orphaned session metadata"},
}

// collectKnownSessionIDs walks the projects directory and returns
// the set of session UUIDs that still own a .jsonl file. The
// orphan scan compares every sibling entry against this set.
func collectKnownSessionIDs(root fs.FS) (map[string]bool, error) {
	known := map[string]bool{}
	projects, err := fs.ReadDir(root, projectsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return known, nil
		}
		return nil, err
	}
	for _, proj := range projects {
		if !proj.IsDir() {
			continue
		}
		entries, err := fs.ReadDir(root, path.Join(projectsDir, proj.Name()))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			known[strings.TrimSuffix(entry.Name(), ".jsonl")] = true
		}
	}
	return known, nil
}

// addItem appends a path to the plan, but only when the path
// actually exists on disk and we can read its size. Missing
// paths are dropped silently, which keeps the dry-run output
// focused on real artifacts. For a directory, the size is
// computed by walking the tree and summing every file inside.
// That walk is cheap compared to actually moving the data, so
// we pay the cost up front to give the user an accurate total.
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
// Entries that fail to stat are skipped, so a directory we can
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
// that the cleanup methods are in place. Once this assertion is
// here, the type system itself prevents anyone from accidentally
// removing a method while leaving the provider partially-cleaning.
var _ contracts.Cleaner = (*Provider)(nil)
