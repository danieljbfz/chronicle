package copilotagent

import (
	"errors"
	"io/fs"

	"github.com/danieljbfz/chronicle/contracts"
)

// adapterName is the string returned by Provider.Name and
// stamped on every StorageVersion this package produces.
// We declare it as a constant so any reference to the
// adapter's user-facing name goes through one source of
// truth.
const adapterName = "copilot-agent"

// sessionStateDir is the subdirectory under the
// adapter's root that holds one directory per session.
// The full path layout is
// <root>/session-state/<sessionID>/events.jsonl.
const sessionStateDir = "session-state"

// currentVersion is the storage-version code chronicle
// stamps on agent sessions today. The agent runtime is
// young enough that we have only seen one shape on disk;
// when it ships breaking changes, a new fingerprint maps
// to a new version code through knownFingerprints below.
//
// The fingerprint mechanism works the same way it does in
// the claude and copilotchat adapters: read a small
// representative slice of the data, hash a stable subset
// of its structure, look up the result in a map. New
// versions land as new map entries.
const currentVersion = "copilot-agent-1"

// detectInDir inspects the agent's session-state directory
// and returns a StorageVersion. The function follows the
// same resilience contract as the other adapters: a missing
// root yields a known-empty result rather than an error,
// because a fresh-install user (or a user who has never
// invoked the agent runtime) has no data here yet, and the
// doctor view should report that gracefully.
//
// The function is package-private. Callers outside the
// package go through (*Provider).Detect, which adds the
// in-memory caching the Provider contract expects.
func detectInDir(root fs.FS) (contracts.StorageVersion, error) {
	_, err := fs.Stat(root, sessionStateDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return contracts.StorageVersion{
				Adapter: adapterName,
				Version: "unknown",
			}, nil
		}
		return contracts.StorageVersion{}, newError("detect", sessionStateDir, err)
	}

	// We have a session-state directory. Until we have a
	// concrete second version of the format to distinguish
	// from, we report the only version we know without
	// computing a fingerprint. The fingerprint hook is
	// here for the moment a second version arrives.
	return contracts.StorageVersion{
		Adapter: adapterName,
		Version: currentVersion,
		Capabilities: contracts.Capabilities{
			ThreadTree:      false,
			EditingSessions: false,
			ToolInvocations: true,
			ModelMetadata:   true,
		},
	}, nil
}
