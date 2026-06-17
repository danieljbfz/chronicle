package composition

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/danieljbfz/chronicle/contracts"
)

// summaryCacheVersion is the on-disk format version. A loader that finds
// a different version discards the file and rebuilds, so a future change
// to the stored shape never has to migrate an old cache.
const summaryCacheVersion = 1

// summaryCacheFilename is the name of the cache document inside the cache
// directory. One file holds every provider's summaries, keyed internally
// by provider, project, and session id.
const summaryCacheFilename = "summaries.json"

// summaryCacheKeySep joins the parts of a cache key. A NUL byte never
// appears in a provider name, a project id, or a session id, so it
// cannot collide with the values it separates.
const summaryCacheKeySep = "\x00"

// summaryCacheEntry is one cached summary plus the stat data that decides
// whether it is still fresh. The size and modification time come from the
// SessionRef the entry was built for, and the fingerprint is the storage
// version the adapter detected. A later lookup whose ref differs on any of
// the three is a miss.
//
// Stat data rather than a content hash is the right key here for two
// reasons. Hashing would have to read the file, which is the exact cost
// the cache exists to avoid. And session files are append-only logs, so
// any change grows the byte size — and the byte size is part of the key —
// which closes the one hole a stat-based key would otherwise have (an
// in-place edit that preserves both size and modification time). The
// fingerprint catches a storage-format change underneath an unchanged
// file. Deleting the cache directory is always safe: a missing entry only
// costs a re-parse.
type summaryCacheEntry struct {
	SizeBytes       int64                    `json:"size_bytes"`
	ModTimeUnixNano int64                    `json:"mod_time_unix_nano"`
	Fingerprint     string                   `json:"fingerprint"`
	Summary         contracts.SessionSummary `json:"summary"`
}

// summaryCacheFile is the JSON document persisted under the cache
// directory, keyed by the stable per-session cache key.
type summaryCacheFile struct {
	Version int                          `json:"version"`
	Entries map[string]summaryCacheEntry `json:"entries"`
}

// summaryCache resolves session summaries from a persisted store,
// re-parsing only the sessions whose files changed. The App owns one,
// loads it lazily on first use, and flushes it once at the end of a
// listing pass. The cache is a hint: a missing, unreadable, or corrupt
// file costs a re-parse, never a wrong answer. An empty path disables
// persistence, which is the shape NewForTest gets unless a test sets a
// cache directory.
//
// The mutex guards the entries map and the dirty flag. The App is a
// shared value, and a TUI loads its sessions and stats screens through
// concurrent commands, so two goroutines can drive a listing at once.
// Without the lock their get and put calls would race on the map, which
// in Go is a panic, not a benign data race.
type summaryCache struct {
	path    string
	mu      sync.Mutex
	entries map[string]summaryCacheEntry
	dirty   bool
}

// loadSummaryCache reads the cache file at path. A missing file, an
// unreadable file, a malformed file, or a version mismatch all yield an
// empty-but-usable cache rather than an error, because the cache only
// ever speeds up a re-parse. An empty path is the persistence-disabled
// shape: the cache works in memory and flush does nothing.
func loadSummaryCache(path string) *summaryCache {
	c := &summaryCache{path: path, entries: map[string]summaryCacheEntry{}}
	if path == "" {
		return c
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	var file summaryCacheFile
	if err := json.Unmarshal(data, &file); err != nil {
		return c
	}
	if file.Version != summaryCacheVersion || file.Entries == nil {
		return c
	}
	c.entries = file.Entries
	return c
}

// get returns the cached summary for ref under the given provider and
// fingerprint when the stored entry matches the ref's size and
// modification time. The bool reports a hit.
func (c *summaryCache) get(provider, fingerprint string, ref contracts.SessionRef) (contracts.SessionSummary, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[summaryCacheKey(provider, ref)]
	if !ok {
		return contracts.SessionSummary{}, false
	}
	if entry.SizeBytes != ref.SizeBytes ||
		entry.ModTimeUnixNano != ref.ModTime.UnixNano() ||
		entry.Fingerprint != fingerprint {
		return contracts.SessionSummary{}, false
	}
	return entry.Summary, true
}

// put stores summary for ref and marks the cache dirty so the next flush
// writes it.
func (c *summaryCache) put(provider, fingerprint string, ref contracts.SessionRef, summary contracts.SessionSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[summaryCacheKey(provider, ref)] = summaryCacheEntry{
		SizeBytes:       ref.SizeBytes,
		ModTimeUnixNano: ref.ModTime.UnixNano(),
		Fingerprint:     fingerprint,
		Summary:         summary,
	}
	c.dirty = true
}

// summaryCacheKey is the stable per-session key: provider, project, and
// id. The size and modification time are stored in the entry and compared
// on lookup rather than folded into the key, so a changed file updates its
// own entry in place instead of leaking a new key on every edit.
func summaryCacheKey(provider string, ref contracts.SessionRef) string {
	return provider + summaryCacheKeySep + string(ref.Project) + summaryCacheKeySep + string(ref.ID)
}

// flush writes the cache to disk when it has unsaved entries and a real
// path. It writes the whole document in one os.WriteFile, the same way the
// trash subsystem writes its manifest. A torn write is harmless because
// loadSummaryCache treats a malformed file as empty and rebuilds. Every
// failure is swallowed: the cache is an optimization, and failing to
// persist it must never fail the listing the user asked for.
func (c *summaryCache) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty || c.path == "" {
		return
	}
	data, err := json.Marshal(summaryCacheFile{Version: summaryCacheVersion, Entries: c.entries})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return
	}
	if err := os.WriteFile(c.path, data, 0o644); err != nil {
		return
	}
	c.dirty = false
}

// summaryCacheHandle returns the App's persistent summary cache, loading
// it from disk on the first call and serving the same instance after. An
// empty cache directory — the NewForTest default — hands the loader an
// empty path, which disables persistence and keeps tests from writing a
// stray file.
func (a *App) summaryCacheHandle() *summaryCache {
	a.cacheOnce.Do(func() {
		path := ""
		if a.locations.CacheDir != "" {
			path = filepath.Join(a.locations.CacheDir, summaryCacheFilename)
		}
		a.cache = loadSummaryCache(path)
	})
	return a.cache
}

// flushSummaryCache persists the cache when one was loaded during the
// call. Entrypoint methods that list sessions defer this so freshly
// parsed summaries survive to the next run.
func (a *App) flushSummaryCache() {
	if a.cache != nil {
		a.cache.flush()
	}
}
