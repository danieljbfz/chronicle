package copilot

import (
	"encoding/json"
	"io/fs"
	"path"
	"strings"

	"github.com/danieljbfz/chronicle/contracts"
)

// This file extends PlanOrphanScan with the floating-junk
// heuristics for Copilot that have nothing to do with a
// specific chat session. Today the only piece it covers is:
//
//   1. Images under globalStorage/github.copilot-chat/copilot-cli-images/
//      whose owning Copilot CLI session is no longer recorded
//      in the CLI's session metadata file. The CLI keeps a
//      small JSON index of its sessions, and any image whose
//      session ID is not in that index is leftover state.
//
// We deliberately do not touch entries inside the metadata
// JSON file itself. Modifying a JSON object instead of moving
// a whole file is a different kind of operation. It would
// require a partial rewrite, which complicates restoration,
// and the file is small enough that the disk savings would
// not be worth the added complexity. If the file becomes a
// problem in the future we can revisit it.

// copilotCLIDir is the subdirectory under globalStorage that
// holds Copilot CLI configuration and session data.
const copilotCLIDir = "globalStorage/github.copilot-chat/copilotCli"

// copilotCLIMetadataFile is the JSON index inside the CLI
// directory that lists the user's Copilot CLI sessions. Each
// top-level key is a session UUID, and each value carries the
// workspace folder, a timestamp, and an optional custom title.
const copilotCLIMetadataFile = "copilotcli.session.metadata.json"

// copilotCLIImagesDir holds images Copilot CLI sessions saved.
// File names start with the session UUID they belong to, so we
// can match each image to a metadata entry by prefix.
const copilotCLIImagesDir = "globalStorage/github.copilot-chat/copilot-cli-images"

// scanFloatingOrphans appends Copilot floating-junk items to
// the plan. The function is called from PlanOrphanScan after
// the per-session orphan checks, so the resulting plan covers
// every kind of orphan the Copilot adapter knows how to find.
func scanFloatingOrphans(root fs.FS, plan *contracts.DeletePlan) {
	scanCLIImageOrphans(root, plan)
}

// scanCLIImageOrphans flags images under copilot-cli-images/
// whose session UUID is not present in the CLI metadata file.
// We read the metadata once to build a set of live session
// IDs, then walk the image directory and flag every file whose
// name does not start with one of those IDs.
//
// If the metadata file is unreadable we leave the images
// alone. Without the reference set we cannot tell which
// images are still in use, and the safe default is to keep
// everything.
func scanCLIImageOrphans(root fs.FS, plan *contracts.DeletePlan) {
	known := readCLISessionIDs(root)
	if known == nil {
		return
	}
	entries, err := fs.ReadDir(root, copilotCLIImagesDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if hasKnownSessionPrefix(name, known) {
			continue
		}
		addItem(root, plan, path.Join(copilotCLIImagesDir, name), "orphaned Copilot CLI image")
	}
}

// readCLISessionIDs returns the set of session UUIDs the
// Copilot CLI metadata file currently lists. The function
// returns nil (not an empty map) when the metadata file is
// missing or unreadable, so the caller can tell the difference
// between "no live sessions" and "could not check".
func readCLISessionIDs(root fs.FS) map[string]bool {
	data, err := fs.ReadFile(root, path.Join(copilotCLIDir, copilotCLIMetadataFile))
	if err != nil {
		return nil
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil
	}
	known := make(map[string]bool, len(metadata))
	for sessionID := range metadata {
		known[sessionID] = true
	}
	return known
}

// hasKnownSessionPrefix reports whether the file name starts
// with any of the known session UUIDs. Image filenames in the
// CLI directory follow the pattern <sessionID>-<rest>, so a
// prefix check is enough to attribute an image to its session.
func hasKnownSessionPrefix(name string, known map[string]bool) bool {
	for sessionID := range known {
		if strings.HasPrefix(name, sessionID) {
			return true
		}
	}
	return false
}
