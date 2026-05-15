package claude

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/danieljbfz/chronicle/contracts"
)

// globalConfigFile is the filename Claude uses for its
// user-wide configuration. The file lives at the top of the
// user's home directory, which is one level above the
// adapter's data root, so the GlobalConfig methods reach
// for it through the homeDir field on Provider rather than
// through the fs.FS argument.
const globalConfigFile = ".claude.json"

// configBackupDir is the subdirectory inside the adapter's
// data root where chronicle places backups of the global
// config file before editing it. Claude's own auto-cleaner
// also rotates files into this directory, so the backups
// chronicle writes blend in with the existing pattern.
const configBackupDir = "backups"

// configBackupPrefix is the filename prefix chronicle uses
// for its own backups. We deliberately match the prefix
// Claude itself uses, so a user listing the backups
// directory sees one consistent timeline of config
// snapshots regardless of who wrote each entry.
const configBackupPrefix = ".claude.json.backup."

// projectsKey is the top-level key inside the global config
// JSON object whose value is the per-project map. Chronicle
// reads and writes only this subsection. Every other
// top-level field (oauthAccount, userID, settings, and so
// on) stays untouched.
const projectsKey = "projects"

// errMissingHomeDir is the explicit signal a Provider built
// with New (instead of NewWithHome) returns from the
// GlobalConfig methods. Production code uses NewWithHome
// via the registry factory, so this branch fires only in
// tests that exercise GlobalConfig without configuring the
// Provider properly.
var errMissingHomeDir = errors.New("claude: provider has no home directory configured (use NewWithHome)")

// ListConfigProjectEntries returns one entry per project
// recorded in ~/.claude.json. The Exists flag tells the
// caller which entries are stale (the directory the key
// names is no longer on disk). The size is the byte length
// of the entry's JSON body, useful for the dry-run output.
//
// The function returns an empty slice (not an error) when
// the global config file is missing or has no projects
// subsection. A fresh-install user hits this branch.
//
// We use gjson to walk the file because it preserves the
// raw bytes of each value, which makes the per-entry size
// measurement exact (it counts the original on-disk bytes
// rather than what a re-encoder would have written).
func (p *Provider) ListConfigProjectEntries(_ fs.FS) ([]contracts.ConfigProjectEntry, error) {
	if p.homeDir == "" {
		return nil, errMissingHomeDir
	}
	configPath := filepath.Join(p.homeDir, globalConfigFile)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read global config: %w", err)
	}
	if !gjson.ValidBytes(data) {
		return nil, fmt.Errorf("global config at %s is not valid JSON", configPath)
	}

	projectsValue := gjson.GetBytes(data, projectsKey)
	if !projectsValue.IsObject() {
		return nil, nil
	}

	var entries []contracts.ConfigProjectEntry
	projectsValue.ForEach(func(key, value gjson.Result) bool {
		entries = append(entries, contracts.ConfigProjectEntry{
			Key:       key.String(),
			Exists:    pathIsDir(key.String()),
			SizeBytes: int64(len(value.Raw)),
		})
		return true
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries, nil
}

// RemoveConfigProjectEntries rewrites ~/.claude.json with
// the named keys removed from the projects subsection. The
// safety pattern is: copy the original to a timestamped
// backup, then build the edited content, then write to a
// temp file in the same directory, then atomically rename
// the temp file over the original. Any failure
// short-circuits without touching the original.
//
// The function returns the absolute path of the backup file
// the user can recover from if they regret the removal.
//
// Keys that are not present in the projects subsection are
// skipped silently. The dry-run plan and the apply step
// happen at different times, and a stale plan should not
// block a real removal of the entries that do still exist.
//
// The edit uses sjson.DeleteBytes, which preserves the
// formatting and key order of the rest of the file. This
// matters because users diff their config history, and a
// chronicle removal that re-indented every line would make
// the diff useless. We also avoid the precision drift that
// would happen if we round-tripped large numeric fields
// (token counts, FPS measurements) through encoding/json's
// default float64 decoder.
func (p *Provider) RemoveConfigProjectEntries(_ fs.FS, keys []string) (string, error) {
	if p.homeDir == "" {
		return "", errMissingHomeDir
	}
	if len(keys) == 0 {
		return "", nil
	}
	configPath := filepath.Join(p.homeDir, globalConfigFile)

	// Step 1: read the current content. We work with the
	// raw bytes throughout so the parts of the file we do
	// not touch stay byte-identical after the write.
	original, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read global config: %w", err)
	}
	if !gjson.ValidBytes(original) {
		return "", fmt.Errorf("global config at %s is not valid JSON", configPath)
	}

	// Step 2: prepare the backup. We do this before any
	// writes so a backup-create failure aborts the whole
	// operation with the original intact. The backup goes
	// next to the existing Claude-rotated backups so the
	// user finds them all in one place.
	backupPath, err := p.backupGlobalConfig(original)
	if err != nil {
		return "", fmt.Errorf("backup global config: %w", err)
	}

	// Step 3: apply the deletions one key at a time. sjson
	// path syntax escapes "." and other separators with a
	// backslash, which sjsonPath builds for us. The keys
	// are absolute filesystem paths that contain slashes
	// but never the literal characters sjson treats as
	// special, so the escaping is straightforward.
	edited := original
	for _, key := range keys {
		next, err := sjson.DeleteBytes(edited, sjsonPath(projectsKey, key))
		if err != nil {
			return backupPath, fmt.Errorf("delete %q: %w", key, err)
		}
		edited = next
	}

	// Step 4: write the edited content to a temp file in
	// the same directory and atomically rename it over the
	// original. Same-directory placement is important
	// because cross-filesystem renames are not atomic on
	// POSIX, and we need atomicity so a crash during the
	// rename leaves either the old file or the new file
	// intact, never a half-written one.
	tempPath := configPath + ".tmp"
	if err := os.WriteFile(tempPath, edited, 0o644); err != nil {
		return backupPath, fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tempPath, configPath); err != nil {
		os.Remove(tempPath) //nolint:errcheck // best-effort cleanup of the temp file we just failed to rename
		return backupPath, fmt.Errorf("rename temp into place: %w", err)
	}
	return backupPath, nil
}

// sjsonPath builds an sjson path from a parent key and a
// child key, escaping dots and stars in the child so
// sjson treats them as literals rather than path
// separators or wildcards. Keys in Claude's projects map
// are absolute filesystem paths, which contain "." in
// directory names like "v1.2.3". Without escaping, sjson
// would interpret the dot as another nesting level and
// fail to find the key.
func sjsonPath(parent, child string) string {
	escaped := strings.ReplaceAll(child, `.`, `\.`)
	escaped = strings.ReplaceAll(escaped, `*`, `\*`)
	return parent + "." + escaped
}

// backupGlobalConfig copies the original bytes to a
// timestamped backup file under <claudeRoot>/backups/. The
// timestamp matches the millisecond-epoch format Claude
// itself uses for its own rotated backups, so the user
// sees one consistent timeline.
func (p *Provider) backupGlobalConfig(original []byte) (string, error) {
	backupsDir := filepath.Join(p.homeDir, ".claude", configBackupDir)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		return "", err
	}
	stamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	backupPath := filepath.Join(backupsDir, configBackupPrefix+stamp)
	if err := os.WriteFile(backupPath, original, 0o644); err != nil {
		return "", err
	}
	return backupPath, nil
}

// pathIsDir reports whether path resolves to a directory on
// the real filesystem. The function is the test for "is
// this project entry stale?" Returning false when the path
// does not exist or is not a directory produces the right
// signal for either case.
//
// We use the OS filesystem directly (not the fs.FS argument)
// because the global config file references absolute paths
// outside the adapter's data root. Stat-ing through fs.FS
// would not see them at all.
func pathIsDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// Compile-time check: *Provider satisfies the optional
// contracts.GlobalConfig capability. If a future contract
// change drops or renames a method, the build fails right
// here with the missing-method error.
var _ contracts.GlobalConfig = (*Provider)(nil)
