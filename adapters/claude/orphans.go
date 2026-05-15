package claude

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/danieljbfz/chronicle/contracts"
)

// This file extends PlanOrphanScan with the floating-junk
// heuristics that have nothing to do with a specific session.
// The pieces it covers are:
//
//   1. paste-cache entries whose hash is not referenced by any
//      record in history.jsonl. These are pasted-text caches
//      Claude saves so the user can recall a paste through the
//      up-arrow history. When the history entry is gone, the
//      cache file is dead weight.
//
//   2. security_warnings_state_<uuid>.json files whose UUID
//      does not match a live session. Claude writes one of
//      these per session that ever showed a security warning.
//      Once the session is gone, the file is leftover state.
//
//   3. shell-snapshots beyond the most recent few. Claude
//      writes one per session start. Old ones are never
//      consulted again.
//
//   4. .claude.json backups beyond the most recent few. Claude
//      rotates these but never trims the tail.
//
// Each heuristic stays defensive. We err on the side of
// keeping a file when we are not sure, because the cost of
// leaving 200 KB on disk is much less than the cost of
// deleting a cache the user actually wanted.

// shellSnapshotsDir holds shell-state snapshots Claude captures
// at session start. The directory grows over time and is rarely
// pruned by Claude itself.
const shellSnapshotsDir = "shell-snapshots"

// pasteCacheDir holds pasted-text caches Claude saves so the
// up-arrow history can recall pastes. Each file is named after
// the content hash of the paste.
const pasteCacheDir = "paste-cache"

// backupsDir holds rotated backups of the top-level .claude.json
// configuration file.
const backupsDir = "backups"

// historyFile is the global prompt history Claude reads to
// reconstruct the up-arrow history. It is a JSONL stream where
// each record may reference paste hashes through a
// pastedContents field.
const historyFile = "history.jsonl"

// shellSnapshotsKeepCount is how many of the most recent shell
// snapshots we keep when scanning. Claude can recreate
// snapshots on demand, so a small retention number is safe.
const shellSnapshotsKeepCount = 5

// backupsKeepCount is how many of the most recent .claude.json
// backups we keep when scanning. Five gives the user a few days
// of rollback room while still allowing the directory to shrink
// over time.
const backupsKeepCount = 5

// scanFloatingOrphans appends floating-junk items to the plan.
// The function is called from PlanOrphanScan after the
// per-session orphan checks, so the resulting plan covers
// everything chronicle knows how to identify as orphan.
func scanFloatingOrphans(root fs.FS, plan *contracts.DeletePlan) {
	scanPasteCacheOrphans(root, plan)
	scanSecurityWarningOrphans(root, plan)
	scanShellSnapshotOrphans(root, plan)
	scanBackupOrphans(root, plan)
}

// scanPasteCacheOrphans flags paste-cache files whose hash is
// not referenced by any record in history.jsonl. We read the
// history file once to build the set of live hashes, then walk
// the cache directory and flag every file whose name (minus
// the .txt extension) is missing from the set.
//
// If the history file is unreadable we leave the cache alone
// entirely. Without the reference set we cannot tell which
// caches are still in use, and the safe default is to keep
// everything.
func scanPasteCacheOrphans(root fs.FS, plan *contracts.DeletePlan) {
	referenced := readPasteHashes(root)
	if referenced == nil {
		return
	}
	entries, err := fs.ReadDir(root, pasteCacheDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}
		hash := strings.TrimSuffix(entry.Name(), ".txt")
		if referenced[hash] {
			continue
		}
		addItem(root, plan, path.Join(pasteCacheDir, entry.Name()), "orphaned paste-cache entry")
	}
}

// readPasteHashes returns the set of paste content hashes that
// history.jsonl currently references. The function returns nil
// (not an empty map) when the history file is missing or
// unreadable, so the caller can tell the difference between
// "no references" and "could not check".
func readPasteHashes(root fs.FS) map[string]bool {
	data, err := fs.ReadFile(root, historyFile)
	if err != nil {
		return nil
	}
	referenced := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		var record struct {
			PastedContents map[string]struct {
				ContentHash string `json:"contentHash"`
			} `json:"pastedContents"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		for _, paste := range record.PastedContents {
			if paste.ContentHash != "" {
				referenced[paste.ContentHash] = true
			}
		}
	}
	return referenced
}

// scanSecurityWarningOrphans flags security_warnings_state JSON
// files at the top level of ~/.claude whose session UUID does
// not match any live session. The file naming scheme is
// security_warnings_state_<uuid>.json, so we extract the UUID
// and check it against the set of live session IDs that
// collectKnownSessionIDs already builds for the per-session
// orphan scan.
func scanSecurityWarningOrphans(root fs.FS, plan *contracts.DeletePlan) {
	known, err := collectKnownSessionIDs(root)
	if err != nil {
		return
	}
	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		uuid, ok := strings.CutPrefix(name, "security_warnings_state_")
		if !ok {
			continue
		}
		uuid, ok = strings.CutSuffix(uuid, ".json")
		if !ok {
			continue
		}
		if known[uuid] {
			continue
		}
		addItem(root, plan, name, "orphaned security warning state")
	}
}

// scanShellSnapshotOrphans flags shell snapshots beyond the
// most recent few. Claude writes one snapshot per session
// start, so the directory grows with use but never shrinks. We
// sort by name (the names embed a sortable timestamp prefix)
// and keep the newest shellSnapshotsKeepCount entries.
//
// Sorting by name instead of mtime is deliberate. The snapshot
// names start with snapshot-zsh-<unix-millis>-<id>, so a name
// sort gives the same order as a timestamp sort while staying
// independent of file-system mtime quirks.
func scanShellSnapshotOrphans(root fs.FS, plan *contracts.DeletePlan) {
	entries, err := fs.ReadDir(root, shellSnapshotsDir)
	if err != nil {
		return
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) <= shellSnapshotsKeepCount {
		return
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names[shellSnapshotsKeepCount:] {
		addItem(root, plan, path.Join(shellSnapshotsDir, name), "old shell snapshot")
	}
}

// scanBackupOrphans flags .claude.json backups beyond the most
// recent few. Claude rotates the backups but never trims the
// tail, so the directory accumulates state over time. The
// names embed a sortable epoch-millis timestamp, so a reverse
// name sort gives newest-first with no need to read mtimes.
func scanBackupOrphans(root fs.FS, plan *contracts.DeletePlan) {
	entries, err := fs.ReadDir(root, backupsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		return
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// The backup names start with .claude.json.backup. so we
		// only consider files that actually look like backups.
		// Anything else in this directory is not ours to touch.
		if !strings.HasPrefix(entry.Name(), ".claude.json.backup.") {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) <= backupsKeepCount {
		return
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names[backupsKeepCount:] {
		addItem(root, plan, path.Join(backupsDir, name), "old configuration backup")
	}
}
