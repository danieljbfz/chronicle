package composition

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

// trashSchemaVersion is stamped into every manifest so future
// versions of chronicle can tell what shape they are reading. We
// only bump this when the on-disk manifest format changes in a
// way that older or newer chronicle versions cannot read directly.
const trashSchemaVersion = "1.0"

// manifestFilename is the name of the JSON file inside each trash
// entry that records what was deleted and where it came from.
const manifestFilename = "manifest.json"

// filesSubdir is the directory inside a trash entry that holds the
// trashed files themselves. We separate them from the manifest so
// the manifest stays at a known path and the file tree underneath
// preserves the original layout.
const filesSubdir = "files"

// TrashEntry describes one trashed deletion plan, ready for the
// user to inspect, restore, or wait out the retention period. The
// fields mirror what gets stored in the manifest, so the JSON
// shape and the Go shape stay in sync.
type TrashEntry struct {
	// ID is the directory name under the trash root, like
	// "20260515-103045-a1b2c3d4". Stable enough to use as a
	// command-line argument to restore or list-detail.
	ID string `json:"id"`

	// SchemaVersion is the manifest format version that produced
	// this entry. Lets future chronicle versions detect entries
	// they cannot safely restore.
	SchemaVersion string `json:"schema_version"`

	// TrashedAt is when the move happened, in UTC.
	TrashedAt time.Time `json:"trashed_at"`

	// Provider is the adapter name that owned the trashed data,
	// like "claude" or "copilot". We need this on restore to
	// pick the right provider root.
	Provider string `json:"provider"`

	// SessionID is the session this entry came from, when one
	// applies. Stays empty for orphan-scan plans.
	SessionID contracts.SessionID `json:"session_id,omitempty"`

	// Category is a human-readable label for what kind of plan
	// produced this entry, like "claude-session" or
	// "claude-orphan-file-history". Helps the trash listing
	// group similar entries.
	Category string `json:"category"`

	// ProviderRoot is the absolute filesystem path the adapter
	// was reading from when the plan was made. We record it so
	// restore can put files back even if the user has changed
	// their config in the meantime.
	ProviderRoot string `json:"provider_root"`

	// Items is the list of paths that were moved into the trash
	// entry. Order matches the original DeletePlan.
	Items []TrashItem `json:"items"`

	// SizeBytes is the total recoverable size, summed over Items.
	SizeBytes int64 `json:"size_bytes"`
}

// TrashItem is one file or directory inside a TrashEntry.
// OriginalPath is where the data lived before the move.
// RelativePath is where it now sits inside the trash entry,
// expressed as a slash-separated path underneath the entry's
// files/ directory.
type TrashItem struct {
	OriginalPath string `json:"original_path"`
	RelativePath string `json:"relative_path"`
	SizeBytes    int64  `json:"size_bytes"`
	Reason       string `json:"reason"`
}

// plannedDeletion pairs a provider-produced DeletePlan with the
// providerEntry that produced it. Keeping the link in one place
// means Trash and the cleanup orchestrator can both reach the
// provider root, name, and FS without re-walking the registry.
type plannedDeletion struct {
	provider *providerEntry
	plan     contracts.DeletePlan
}

// Trash moves every item in the plan into a fresh entry under
// the chronicle trash directory and returns the resulting
// TrashEntry. We aim for all-or-nothing behaviour. If a later
// item in the plan fails to move, we put the earlier items back
// where they came from and return an error. That avoids leaving
// the user's data in a half-deleted state where some files are
// in the trash and others are still in place.
//
// One kind of failure does not trigger a rollback. If an item's
// source file has gone missing between the moment the plan was
// built and the moment we try to move it (the user might have
// deleted it manually), we skip that one item and keep going.
// The plan is a snapshot, not a lock, so a missing source is a
// normal race rather than a bug.
func (a *App) Trash(planned plannedDeletion) (TrashEntry, error) {
	if planned.provider == nil {
		return TrashEntry{}, errors.New("composition.Trash: planned deletion has no provider")
	}

	entryID, err := newTrashEntryID(time.Now().UTC())
	if err != nil {
		return TrashEntry{}, fmt.Errorf("composition.Trash: generate id: %w", err)
	}

	entryDir := filepath.Join(a.locations.TrashDir, entryID)
	filesDir := filepath.Join(entryDir, filesSubdir)
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		return TrashEntry{}, fmt.Errorf("composition.Trash: create entry dir: %w", err)
	}

	entry := TrashEntry{
		ID:            entryID,
		SchemaVersion: trashSchemaVersion,
		TrashedAt:     time.Now().UTC(),
		Provider:      planned.provider.Provider.Name(),
		SessionID:     planned.plan.SessionID,
		Category:      planned.plan.Category,
		ProviderRoot:  planned.provider.Root,
	}

	// Step 1: move each item. We track the moves we have done so
	// we can roll them back if a later item fails.
	var completed []completedMove

	for _, item := range planned.plan.Items {
		original := filepath.Join(planned.provider.Root, item.Path)
		if _, err := os.Lstat(original); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// The file vanished between plan-time and now.
				// Skip it; do not abort the whole move.
				continue
			}
			rollbackMoves(completed)
			os.RemoveAll(entryDir) //nolint:errcheck // best-effort cleanup after a real failure
			return TrashEntry{}, fmt.Errorf("composition.Trash: stat %s: %w", original, err)
		}

		relative := filepath.ToSlash(item.Path)
		trashed := filepath.Join(filesDir, item.Path)
		if err := os.MkdirAll(filepath.Dir(trashed), 0o755); err != nil {
			rollbackMoves(completed)
			os.RemoveAll(entryDir) //nolint:errcheck
			return TrashEntry{}, fmt.Errorf("composition.Trash: prepare destination: %w", err)
		}
		if err := moveFileOrDir(original, trashed); err != nil {
			rollbackMoves(completed)
			os.RemoveAll(entryDir) //nolint:errcheck
			return TrashEntry{}, fmt.Errorf("composition.Trash: move %s: %w", original, err)
		}
		completed = append(completed, completedMove{original: original, trashed: trashed})

		entry.Items = append(entry.Items, TrashItem{
			OriginalPath: original,
			RelativePath: relative,
			SizeBytes:    item.SizeBytes,
			Reason:       item.Reason,
		})
		entry.SizeBytes += item.SizeBytes
	}

	// Step 2: write the manifest. If this fails, roll the moves
	// back so the trash directory does not contain an entry
	// without a manifest (which would be unrestorable).
	if err := writeManifest(entryDir, entry); err != nil {
		rollbackMoves(completed)
		os.RemoveAll(entryDir) //nolint:errcheck
		return TrashEntry{}, fmt.Errorf("composition.Trash: write manifest: %w", err)
	}

	return entry, nil
}

// TrashList returns every entry currently sitting in the trash
// directory, sorted newest first. The CLI uses this for the
// `chronicle trash list` output, and any future UI can use it
// for the same purpose.
//
// Entries with a missing or malformed manifest get skipped
// silently. A single broken entry should not hide every other
// recoverable entry from the user, so we trade strictness for
// usability here. A future doctor view can surface the broken
// entries as warnings if that becomes useful.
func (a *App) TrashList() ([]TrashEntry, error) {
	if err := os.MkdirAll(a.locations.TrashDir, 0o755); err != nil {
		return nil, fmt.Errorf("composition.TrashList: ensure trash dir: %w", err)
	}

	entries, err := os.ReadDir(a.locations.TrashDir)
	if err != nil {
		return nil, fmt.Errorf("composition.TrashList: read trash dir: %w", err)
	}

	var out []TrashEntry
	for _, dirent := range entries {
		if !dirent.IsDir() {
			continue
		}
		entry, err := readManifest(filepath.Join(a.locations.TrashDir, dirent.Name()))
		if err != nil {
			// Skip unreadable entries. A future doctor view can
			// surface these as warnings; for now we silently move
			// past them so the user sees what is restorable.
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].TrashedAt.After(out[j].TrashedAt)
	})
	return out, nil
}

// TrashRestore moves every item in the named entry back to its
// original location. It refuses to overwrite an existing file
// at the destination, because doing so could silently destroy
// newer data the user wrote after the trash operation. The
// check happens up front: if any single item's destination
// already exists, the whole restore aborts before moving
// anything, so the user never ends up with a partial restore.
func (a *App) TrashRestore(id string) error {
	entryDir := filepath.Join(a.locations.TrashDir, id)
	entry, err := readManifest(entryDir)
	if err != nil {
		return fmt.Errorf("composition.TrashRestore: read manifest: %w", err)
	}

	// Step 1: pre-flight every destination. If any one already
	// exists, abort before we move anything.
	for _, item := range entry.Items {
		if _, err := os.Lstat(item.OriginalPath); err == nil {
			return fmt.Errorf("composition.TrashRestore: destination already exists: %s", item.OriginalPath)
		}
	}

	// Step 2: move every item back. If a later move fails after
	// some have already gone back, we leave the rest in place
	// and report the partial state. Restore is opt-in and rare
	// enough that asking the user to investigate manually is
	// fine.
	for _, item := range entry.Items {
		source := filepath.Join(entryDir, filesSubdir, filepath.FromSlash(item.RelativePath))
		if err := os.MkdirAll(filepath.Dir(item.OriginalPath), 0o755); err != nil {
			return fmt.Errorf("composition.TrashRestore: prepare %s: %w", item.OriginalPath, err)
		}
		if err := moveFileOrDir(source, item.OriginalPath); err != nil {
			return fmt.Errorf("composition.TrashRestore: move back %s: %w", item.OriginalPath, err)
		}
	}

	// Step 3: drop the now-empty entry directory. Best effort:
	// if it fails (the user has files there we did not put), we
	// leave it for them to investigate.
	os.RemoveAll(entryDir) //nolint:errcheck
	return nil
}

// TrashEmptyOptions controls which trash entries TrashEmpty
// removes. The default behaviour is to remove only entries older
// than the retention window from the user's config (typically 30
// days). Setting Force=true removes everything regardless of age.
type TrashEmptyOptions struct {
	// Force, when true, ignores the retention window and removes
	// every entry. Use sparingly; this is the only command in
	// chronicle that performs an unrecoverable delete.
	Force bool

	// Now overrides the current time, used by tests to make
	// retention checks deterministic. Production code leaves
	// this as the zero value.
	Now time.Time
}

// TrashEmpty permanently removes trash entries that are
// eligible for removal. An entry is eligible when it is older
// than the retention window in the user's config, or when the
// caller passes Force=true to ignore the window. The function
// returns the IDs of the entries it removed, so the caller can
// show the user a confirmation list.
//
// This is the only command in chronicle that performs a
// non-recoverable delete. The default retention window of 30
// days is deliberately generous, because the cost of an
// accidental wipe is high and the cost of holding on to old
// trash entries for a few extra weeks is essentially zero.
func (a *App) TrashEmpty(opts TrashEmptyOptions) ([]string, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	retention := time.Duration(a.settings.Trash.RetentionDays) * 24 * time.Hour

	entries, err := a.TrashList()
	if err != nil {
		return nil, err
	}

	var removed []string
	for _, entry := range entries {
		if !opts.Force && now.Sub(entry.TrashedAt) < retention {
			continue
		}
		entryDir := filepath.Join(a.locations.TrashDir, entry.ID)
		if err := os.RemoveAll(entryDir); err != nil {
			return removed, fmt.Errorf("composition.TrashEmpty: remove %s: %w", entry.ID, err)
		}
		removed = append(removed, entry.ID)
	}
	return removed, nil
}

// newTrashEntryID builds a sortable, collision-resistant entry
// directory name from the given timestamp. The format is
// YYYYMMDDHHMMSS-<8 hex chars of randomness>, so two entries
// created in the same second still get distinct IDs.
func newTrashEntryID(now time.Time) (string, error) {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s",
		now.Format("20060102-150405"),
		hex.EncodeToString(random[:]),
	), nil
}

// completedMove records one already-finished file move so the
// rollback path can put the file back if a later move in the
// same plan fails. Unexported because nothing outside this file
// has a use for it.
type completedMove struct {
	original string
	trashed  string
}

// rollbackMoves walks the completed-moves slice in reverse
// order and tries to put every file back where it came from. We
// accept best-effort behaviour here. If a rollback move itself
// fails, which is rare because the source path should still be
// free, there is not much we can do beyond surfacing the
// original failure to the user and letting them investigate.
func rollbackMoves(completed []completedMove) {
	for i := len(completed) - 1; i >= 0; i-- {
		c := completed[i]
		_ = moveFileOrDir(c.trashed, c.original)
	}
}

// writeManifest serializes the entry as JSON and writes it to
// the entry directory. The JSON is indented because a curious
// user might want to `cat` the manifest, and indented JSON
// reads more naturally than a single long line.
func writeManifest(entryDir string, entry TrashEntry) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(entryDir, manifestFilename), data, 0o644)
}

// readManifest deserializes the manifest at the entry directory
// back into a TrashEntry. The function also fills in the entry
// ID from the directory name when the manifest itself does not
// carry one, so callers always get a usable ID back.
func readManifest(entryDir string) (TrashEntry, error) {
	data, err := os.ReadFile(filepath.Join(entryDir, manifestFilename))
	if err != nil {
		return TrashEntry{}, err
	}
	var entry TrashEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return TrashEntry{}, err
	}
	if entry.ID == "" {
		entry.ID = filepath.Base(entryDir)
	}
	return entry, nil
}

// String formats a TrashEntry as a human-readable single line for
// the trash list. Useful for command-line output. Format:
// "<id>  <provider>:<short session id>  <size>  (<age>)"
func (e TrashEntry) String() string {
	short := string(e.SessionID)
	if len(short) > 8 {
		short = short[:8]
	}
	if short == "" {
		short = "(orphan scan)"
	}
	return fmt.Sprintf("%s  %s:%s  %s  %s",
		e.ID,
		e.Provider,
		short,
		humanBytes(e.SizeBytes),
		humanAge(e.TrashedAt),
	)
}
