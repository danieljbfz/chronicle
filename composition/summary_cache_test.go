package composition

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

func cacheRef(id string, size int64, mod time.Time) contracts.SessionRef {
	return contracts.SessionRef{ID: contracts.SessionID(id), Project: "proj", SizeBytes: size, ModTime: mod}
}

// TestSummaryCache_hitWhenSizeAndModTimeMatch is the happy path: a stored
// summary comes back when the ref it was stored for is unchanged.
func TestSummaryCache_hitWhenSizeAndModTimeMatch(t *testing.T) {
	c := loadSummaryCache(filepath.Join(t.TempDir(), "summaries.json"))
	ref := cacheRef("s1", 10, time.Unix(1000, 0))

	c.put("claude", "fp1", ref, contracts.SessionSummary{ID: "s1", Title: "hello"})
	got, ok := c.get("claude", "fp1", ref)

	if !ok || got.Title != "hello" {
		t.Fatalf("expected hit, got ok=%v summary=%+v", ok, got)
	}
}

// TestSummaryCache_missWhenIdentityChanges confirms the cache treats a
// changed size, a changed modification time, or a changed storage
// fingerprint as stale and re-parses rather than serving the old summary.
func TestSummaryCache_missWhenIdentityChanges(t *testing.T) {
	c := loadSummaryCache(filepath.Join(t.TempDir(), "summaries.json"))
	mod := time.Unix(1000, 0)
	c.put("claude", "fp1", cacheRef("s1", 10, mod), contracts.SessionSummary{ID: "s1"})

	if _, ok := c.get("claude", "fp1", cacheRef("s1", 11, mod)); ok {
		t.Error("a changed size should miss")
	}
	if _, ok := c.get("claude", "fp1", cacheRef("s1", 10, time.Unix(2000, 0))); ok {
		t.Error("a changed modification time should miss")
	}
	if _, ok := c.get("claude", "fp2", cacheRef("s1", 10, mod)); ok {
		t.Error("a changed fingerprint should miss")
	}
}

// TestSummaryCache_persistsAcrossLoad confirms a flushed cache is served
// back by a fresh load, which is the whole point: a later process skips
// the parse. It round-trips every field a summary carries — including the
// timestamps and the nested Source — so a serialization regression that
// dropped or corrupted a field cannot pass. Timestamps are compared with
// Equal, which compares the instant: JSON normalizes a time's zone to a
// fixed offset, so the wall-clock instant is what has to survive, not the
// time.Location identity.
func TestSummaryCache_persistsAcrossLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summaries.json")
	ref := cacheRef("s1", 10, time.Unix(1000, 0))
	want := contracts.SessionSummary{
		ID:           "s1",
		Project:      "proj",
		StartedAt:    time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC),
		LastActive:   time.Date(2026, 6, 1, 10, 30, 0, 0, time.UTC),
		Title:        "kept",
		TurnCount:    7,
		SizeBytes:    10,
		Model:        "claude-opus-4",
		Capabilities: contracts.Capabilities{ModelMetadata: true},
		Source:       contracts.StorageVersion{Adapter: "claude", Version: "claude-1.0", Fingerprint: "fp1"},
	}

	first := loadSummaryCache(path)
	first.put("claude", "fp1", ref, want)
	first.flush()

	got, ok := loadSummaryCache(path).get("claude", "fp1", ref)
	if !ok {
		t.Fatal("expected a persisted hit after reload")
	}
	if got.ID != want.ID || got.Project != want.Project || got.Title != want.Title ||
		got.TurnCount != want.TurnCount || got.SizeBytes != want.SizeBytes || got.Model != want.Model {
		t.Errorf("scalar fields did not survive the round-trip: got %+v", got)
	}
	if !got.StartedAt.Equal(want.StartedAt) || !got.LastActive.Equal(want.LastActive) {
		t.Errorf("timestamps did not survive: started %v / active %v", got.StartedAt, got.LastActive)
	}
	if got.Capabilities != want.Capabilities || got.Source != want.Source {
		t.Errorf("capabilities or source did not survive: caps %+v source %+v", got.Capabilities, got.Source)
	}
}

// TestSummaryCache_corruptFileRebuildsEmpty confirms a damaged cache file
// behaves as an empty cache rather than failing the listing.
func TestSummaryCache_corruptFileRebuildsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summaries.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := loadSummaryCache(path)
	if _, ok := c.get("claude", "fp1", cacheRef("s1", 10, time.Unix(1000, 0))); ok {
		t.Fatal("a corrupt cache should behave as empty")
	}
}

// TestSummaryCache_emptyPathDisablesPersistence confirms the NewForTest
// shape: with no cache directory the cache works in memory and flush is a
// no-op, so a test never writes a stray file to the working directory.
func TestSummaryCache_emptyPathDisablesPersistence(t *testing.T) {
	c := loadSummaryCache("")
	ref := cacheRef("s1", 10, time.Unix(1000, 0))

	c.put("claude", "fp1", ref, contracts.SessionSummary{ID: "s1"})
	c.flush() // must not panic or write anywhere

	if _, ok := c.get("claude", "fp1", ref); !ok {
		t.Fatal("an in-memory cache should still serve within the process")
	}
}
