package copilotchat

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/steps"
)

// adapterName is the string returned by Provider.Name and stamped
// on every StorageVersion this package produces.
const adapterName = "copilot-chat"

// maxFingerprintRecords caps how many event records contribute to
// the fingerprint. The first records carry the schema variety the
// fingerprint cares about, so reading the whole event log would be
// wasteful on a session with thousands of patches.
const maxFingerprintRecords = 200

// knownFingerprints maps fingerprints we have seen on real Copilot
// data to internal version codes. New entries land here as we
// observe new VS Code releases. A fingerprint that is not in this
// map produces Version equal to "unknown", and the rest of
// chronicle stays in read-only mode for that storage.
//
// The map starts empty and grows as we encounter new VS Code
// releases. The first entry came from running chronicle against
// real VS Code data on the author's machine.
var knownFingerprints = map[string]string{
	// Captured against VS Code with Copilot Chat schema version 3
	// on 2026-05-15.
	"2e10591741e1": "copilot-3",
}

// detectInDir computes the fingerprint and resulting StorageVersion
// for the first session file it finds under the given root. We
// prefer a session file from workspaceStorage, but fall back to one
// from globalStorage/emptyWindowChatSessions if no workspace
// session is available.
func detectInDir(root fs.FS) (contracts.StorageVersion, error) {
	file, err := firstSessionFile(root)
	if errors.Is(err, fs.ErrNotExist) {
		// No Copilot data here yet. The doctor view will simply
		// show "no Copilot data found" for this root.
		return contracts.StorageVersion{
			Adapter: adapterName,
			Version: "unknown",
		}, nil
	}
	if err != nil {
		return contracts.StorageVersion{}, err
	}

	inputs, parseable, err := readFingerprintInputs(root, file)
	if err != nil {
		return contracts.StorageVersion{}, err
	}
	if !parseable {
		// The file existed but every line failed JSON decoding.
		// The resilience contract says this is the one shape that
		// does count as an error, because we are clearly not
		// pointed at a Copilot session file at all.
		return contracts.StorageVersion{}, newError("detect", file, errors.New("no parseable JSON records"))
	}

	fp := steps.Fingerprint(inputs)
	version, known := knownFingerprints[fp]
	if !known {
		version = "unknown"
	}
	return contracts.StorageVersion{
		Adapter:     adapterName,
		Version:     version,
		Fingerprint: fp,
		Capabilities: contracts.Capabilities{
			ThreadTree:      false, // Copilot stores requests as a flat list, not a tree.
			EditingSessions: known,
			ToolInvocations: known,
			ModelMetadata:   known,
		},
	}, nil
}

// firstSessionFile finds the first Copilot session file under the
// given root. We walk workspaceStorage first because that is where
// most chats live. If we find nothing there, we fall back to the
// empty-window chats under globalStorage. Either way, we return
// fs.ErrNotExist when there is no Copilot session anywhere.
func firstSessionFile(root fs.FS) (string, error) {
	if file, ok := firstWorkspaceSessionFile(root); ok {
		return file, nil
	}
	if file, ok := firstEmptyWindowSessionFile(root); ok {
		return file, nil
	}
	return "", fs.ErrNotExist
}

func firstWorkspaceSessionFile(root fs.FS) (string, bool) {
	workspaces, err := fs.ReadDir(root, workspaceStorageDir)
	if err != nil {
		return "", false
	}
	for _, ws := range workspaces {
		if !ws.IsDir() {
			continue
		}
		dir := path.Join(workspaceStorageDir, ws.Name(), chatSessionsDir)
		entries, err := fs.ReadDir(root, dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
				return path.Join(dir, entry.Name()), true
			}
		}
	}
	return "", false
}

func firstEmptyWindowSessionFile(root fs.FS) (string, bool) {
	entries, err := fs.ReadDir(root, emptyWindowChatSessionsDir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			return path.Join(emptyWindowChatSessionsDir, entry.Name()), true
		}
	}
	return "", false
}

// readFingerprintInputs streams the JSONL file and produces one
// FingerprintInput per recognized event line. The split between
// readFingerprintInputs (which handles the file) and
// collectFingerprintInputs (which handles the stream) keeps the
// I/O part trivial and the parsing part testable on its own.
func readFingerprintInputs(root fs.FS, file string) (inputs []steps.FingerprintInput, parseable bool, err error) {
	f, err := root.Open(file)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	return collectFingerprintInputs(f)
}

// collectFingerprintInputs reads JSON values from the stream and
// extracts the (kind, key-set) tuples the fingerprinter needs. For
// a snapshot event (kind 0), the key set is the keys of the
// snapshot itself. For mutation events (kinds 1 and 2), the key
// set is empty and the kind alone identifies the shape.
//
// We use a streaming JSON decoder instead of reading the file
// line by line. The reason is the same as in eventlog.go: real
// Copilot sessions can have individual events that reach hundreds
// of megabytes, and a line buffer would not survive them.
func collectFingerprintInputs(r io.Reader) ([]steps.FingerprintInput, bool, error) {
	decoder := json.NewDecoder(r)

	var inputs []steps.FingerprintInput
	parseable := false
	for len(inputs) < maxFingerprintRecords {
		var event struct {
			Kind int             `json:"kind"`
			V    json.RawMessage `json:"v"`
		}
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			// We hit a record that will not decode. Rather than
			// give up entirely, we stop here and return the
			// fingerprint we built from the records that did
			// decode. The caller can then decide whether that
			// partial result is enough to identify the storage
			// version.
			break
		}
		parseable = true

		// We label the kind with a string prefix so the fingerprint
		// is easy to recognize when a human looks at it. "kind:0"
		// is the snapshot, "kind:1" is set, "kind:2" is append.
		typeLabel := "kind:" + strconv.Itoa(event.Kind)

		var keys []string
		if event.Kind == snapshotEventKind {
			keys = topLevelKeys(event.V)
		}
		sort.Strings(keys)
		inputs = append(inputs, steps.FingerprintInput{Type: typeLabel, Keys: keys})
	}
	return inputs, parseable, nil
}

// topLevelKeys decodes the snapshot value into a generic map and
// returns its top-level key names. We do not care about the values
// here. The fingerprint is just a hash of the schema shape.
func topLevelKeys(raw json.RawMessage) []string {
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil
	}
	keys := make([]string, 0, len(snapshot))
	for k := range snapshot {
		keys = append(keys, k)
	}
	return keys
}
