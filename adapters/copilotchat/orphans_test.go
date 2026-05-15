package copilotchat

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/danieljbfz/chronicle/contracts"
)

// liveSessionUUID and otherSessionUUID are the two session
// identifiers the orphan tests reference. Keeping them as
// named constants makes the fixtures self-documenting and
// keeps the assertions below from sprinkling raw UUIDs
// around the test bodies.
const (
	liveSessionUUID  = "11111111-1111-1111-1111-111111111111"
	otherSessionUUID = "22222222-2222-2222-2222-222222222222"
)

// metadataFixture is the JSON content of the Copilot CLI
// metadata file the scanner reads. The shape mirrors what
// the real CLI writes: a top-level object keyed by session
// UUID with arbitrary per-session metadata. We include one
// session so the scanner has a non-empty live-session set
// to compare against.
const metadataFixture = `{"` + liveSessionUUID + `":{"workspace":"/Users/me/proj","timestamp":"2026-05-15T10:00:00Z"}}`

// TestHasKnownSessionPrefix_matchesByExactPrefix is the
// happy path. An image filename built from a live session
// UUID followed by a dash and a tail should be attributed
// to that session and excluded from the orphan plan. We
// test the helper directly because the prefix-match rule
// is the contract between the file naming convention and
// the orphan logic, and a regression here would silently
// flag every image as orphan.
func TestHasKnownSessionPrefix_matchesByExactPrefix(t *testing.T) {
	known := map[string]bool{liveSessionUUID: true}
	got := hasKnownSessionPrefix(liveSessionUUID+"-screenshot.png", known)
	if !got {
		t.Errorf("hasKnownSessionPrefix(known prefix) = false, want true")
	}
}

// TestHasKnownSessionPrefix_rejectsUnknownPrefix is the
// negation. An image whose name starts with a UUID the
// metadata file does not list belongs to a session that has
// been deleted, so the function should return false to mark
// it for the orphan plan.
func TestHasKnownSessionPrefix_rejectsUnknownPrefix(t *testing.T) {
	known := map[string]bool{liveSessionUUID: true}
	got := hasKnownSessionPrefix(otherSessionUUID+"-screenshot.png", known)
	if got {
		t.Errorf("hasKnownSessionPrefix(unknown prefix) = true, want false")
	}
}

// TestHasKnownSessionPrefix_emptyKnownSetRejectsEverything
// covers the "no live sessions" edge. When the metadata
// file is empty or every session has been deleted, every
// image becomes an orphan candidate. The helper has to
// agree.
func TestHasKnownSessionPrefix_emptyKnownSetRejectsEverything(t *testing.T) {
	if hasKnownSessionPrefix("anything.png", map[string]bool{}) {
		t.Error("empty known set should reject every name, but accepted one")
	}
}

// TestReadCLISessionIDs_parsesValidMetadata pins the happy
// path of the metadata reader. A valid JSON object with one
// session key produces a one-entry map. The returned map
// must be non-nil even with one entry, because the scanner
// branches on nil to mean "could not check" — the empty
// map case has different semantics.
func TestReadCLISessionIDs_parsesValidMetadata(t *testing.T) {
	fsys := fstest.MapFS{
		copilotCLIDir + "/" + copilotCLIMetadataFile: {Data: []byte(metadataFixture)},
	}
	got := readCLISessionIDs(fsys)
	if got == nil {
		t.Fatal("readCLISessionIDs returned nil for valid metadata")
	}
	if !got[liveSessionUUID] {
		t.Errorf("live session UUID missing from result: %v", got)
	}
}

// TestReadCLISessionIDs_missingMetadataReturnsNil pins the
// "could not check" signal. The scanner uses the nil return
// to mean "leave the images alone" rather than "every image
// is an orphan." A fresh install with no Copilot CLI
// activity yet hits this branch.
func TestReadCLISessionIDs_missingMetadataReturnsNil(t *testing.T) {
	got := readCLISessionIDs(fstest.MapFS{})
	if got != nil {
		t.Errorf("readCLISessionIDs(empty fs) = %v, want nil", got)
	}
}

// TestReadCLISessionIDs_malformedJSONReturnsNil covers the
// corruption case. A metadata file the user (or a tool)
// truncated mid-write is not safe to act on. The scanner
// should fall back to "could not check" instead of treating
// a parse error as "no live sessions" and flagging every
// image for deletion.
func TestReadCLISessionIDs_malformedJSONReturnsNil(t *testing.T) {
	fsys := fstest.MapFS{
		copilotCLIDir + "/" + copilotCLIMetadataFile: {Data: []byte("{not valid json")},
	}
	got := readCLISessionIDs(fsys)
	if got != nil {
		t.Errorf("readCLISessionIDs(malformed) = %v, want nil", got)
	}
}

// TestScanCLIImageOrphans_flagsOrphansAndKeepsLive is the
// end-to-end scan. Two images under the CLI images
// directory: one belongs to a live session and must not
// appear in the plan, the other belongs to a session that
// is no longer in metadata and must appear with the right
// reason string.
func TestScanCLIImageOrphans_flagsOrphansAndKeepsLive(t *testing.T) {
	fsys := fstest.MapFS{
		copilotCLIDir + "/" + copilotCLIMetadataFile:                   {Data: []byte(metadataFixture)},
		copilotCLIImagesDir + "/" + liveSessionUUID + "-keep.png":      {Data: []byte("png-bytes")},
		copilotCLIImagesDir + "/" + otherSessionUUID + "-orphan.png":   {Data: []byte("png-bytes")},
		copilotCLIImagesDir + "/" + otherSessionUUID + "-orphan-2.png": {Data: []byte("png-bytes")},
	}

	var plan contracts.DeletePlan
	scanCLIImageOrphans(fsys, &plan)

	if len(plan.Items) != 2 {
		t.Fatalf("plan items = %d, want 2 (the two unknown-prefix images)", len(plan.Items))
	}
	for _, item := range plan.Items {
		if !strings.Contains(item.Path, otherSessionUUID) {
			t.Errorf("plan item %q should belong to the unknown session", item.Path)
		}
		if item.Reason != reasonOrphanCLIImage {
			t.Errorf("plan item reason = %q, want %q", item.Reason, reasonOrphanCLIImage)
		}
	}
}

// TestScanCLIImageOrphans_doesNothingWithoutMetadata pins
// the safety rule. When the metadata file is missing,
// scanCLIImageOrphans must not produce any plan items. The
// alternative (treating missing metadata as "no live
// sessions, all images are orphans") would wipe every
// Copilot CLI image on a fresh install. The test guards
// against a regression that flips the early-return.
func TestScanCLIImageOrphans_doesNothingWithoutMetadata(t *testing.T) {
	fsys := fstest.MapFS{
		copilotCLIImagesDir + "/" + otherSessionUUID + "-image.png": {Data: []byte("png")},
	}
	var plan contracts.DeletePlan
	scanCLIImageOrphans(fsys, &plan)

	if len(plan.Items) != 0 {
		t.Errorf("plan items = %d, want 0 (no metadata means no scan)", len(plan.Items))
	}
}

// TestScanCLIImageOrphans_skipsDirectoryEntries confirms
// the scanner only flags files. The CLI is unlikely to
// ever create subdirectories under the images folder, but
// if a future version does, we should leave them alone
// rather than try to move a directory through a
// file-oriented plan.
func TestScanCLIImageOrphans_skipsDirectoryEntries(t *testing.T) {
	fsys := fstest.MapFS{
		copilotCLIDir + "/" + copilotCLIMetadataFile: {Data: []byte(metadataFixture)},
		// MapFS interprets paths with a trailing slash as
		// directory implicitly, but the cleanest way to
		// model "directory at this name" is to put a child
		// inside it.
		copilotCLIImagesDir + "/" + otherSessionUUID + "-subdir/inside.txt": {Data: []byte("")},
	}
	var plan contracts.DeletePlan
	scanCLIImageOrphans(fsys, &plan)

	if len(plan.Items) != 0 {
		t.Errorf("plan items = %d, want 0 (a directory named like an orphan should not be flagged)", len(plan.Items))
	}
}
