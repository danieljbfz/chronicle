package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/steps"
)

// adapterName is the string returned by Provider.Name and stamped on
// every StorageVersion this package produces. We declare it as a
// constant so any reference to "claude" inside the package goes
// through a single source of truth and renaming the adapter would be
// one change.
const adapterName = "claude"

// maxFingerprintRecords caps how many records contribute to a
// fingerprint. The first records carry every record type the file
// uses in practice, so reading more would not change the hash.
// Reading a 22 megabyte session file from end to end just to
// fingerprint it would be wasteful when the first two hundred lines
// are already enough.
const maxFingerprintRecords = 200

// projectsDir is the subdirectory under the Claude root that holds
// the per-cwd session folders. We declare it as a constant so the
// detection code and the listing code refer to the same string.
const projectsDir = "projects"

// scannerBufferMax raises the per-line buffer ceiling for
// bufio.Scanner. A single Claude assistant turn with thinking and
// many tool uses can easily exceed bufio.Scanner's default 64 KiB
// per-line limit, and crashing on a long line would be exactly the
// wrong kind of failure for a tool whose job is to read other tools'
// data. Sixteen megabytes is generous and harmless.
const scannerBufferMax = 16 * 1024 * 1024

// knownFingerprints maps detected fingerprints to internal version
// codes. New entries land in this map as the upstream format evolves.
// When chronicle sees a fingerprint that is not in this map, it
// returns Version equal to "unknown" and the rest of the system
// degrades gracefully: read-only access still works through the
// tolerant parser, and destructive operations require an extra
// confirmation.
//
// The map is populated empirically. The very first time chronicle
// runs against a new Claude Code version on a contributor's machine,
// the doctor command prints the fingerprint we observed and we add
// it here in a follow-up commit.
var knownFingerprints = map[string]string{
	// Captured against Claude Code 2.1.x on 2026-05-15.
	"25ce9fd0794c": "claude-1.0",
}

// detectInDir computes the fingerprint and the resulting
// StorageVersion for the first session file it finds under the given
// root. It is the building block that the Provider.Detect method
// wraps, and it is exported only inside the package so tests can call
// it without going through the Provider.
//
// We trust the first session file we find, because every session in
// the same Claude install was written by the same Claude Code
// version. The fingerprints would all agree, so reading the first one
// is enough. If a future Claude Code version starts mixing storage
// formats inside the same install, we would notice during detection
// and pick a different strategy then.
func detectInDir(root fs.FS) (contracts.StorageVersion, error) {
	file, err := firstSessionFile(root)
	if errors.Is(err, fs.ErrNotExist) {
		// There is no projects directory, or the directory is
		// empty. This is not an error. Chronicle should still
		// load, and the doctor view will show "no Claude data
		// found here yet" for this provider.
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
		// pointed at a version of Claude's storage at all.
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
			ThreadTree:      known,
			ToolInvocations: known,
			// Claude records a model identifier on every
			// assistant record, and parse.go reports the
			// most-frequent value as the session-level model.
			// The capability is gated on a recognized
			// fingerprint, the same as the two above, so an
			// unknown storage version makes no claim about
			// what its records contain.
			ModelMetadata: known,
		},
	}, nil
}

// firstSessionFile walks the projects directory looking for the
// first .jsonl file under any subdirectory. It returns fs.ErrNotExist
// when there is no projects directory at all or when there are no
// session files anywhere underneath it.
//
// We use the path package, not path/filepath, because fs.FS always
// uses forward slashes regardless of the operating system. The two
// packages have very similar names but very different jobs:
// path/filepath is for real OS paths and respects the platform's
// separator, while path is for slash-separated paths used by URLs and
// by fs.FS.
func firstSessionFile(root fs.FS) (string, error) {
	projects, err := fs.ReadDir(root, projectsDir)
	if err != nil {
		return "", err
	}
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		entries, err := fs.ReadDir(root, path.Join(projectsDir, p.Name()))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				return path.Join(projectsDir, p.Name(), e.Name()), nil
			}
		}
	}
	return "", fs.ErrNotExist
}

// readFingerprintInputs opens a JSONL file and returns up to
// maxFingerprintRecords (type, keys) tuples for the fingerprinter. The
// parseable boolean becomes true the moment any line decodes
// successfully as JSON. The function is split out from
// collectFingerprintInputs so the file-handling part stays trivial:
// open, defer the close, hand the reader to the parsing helper.
func readFingerprintInputs(root fs.FS, file string) (inputs []steps.FingerprintInput, parseable bool, err error) {
	f, err := root.Open(file)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	return collectFingerprintInputs(f)
}

// collectFingerprintInputs reads JSONL lines from r and produces one
// FingerprintInput per line, skipping any line that fails JSON
// decoding. Skipping the bad lines is deliberate: the resilience
// contract says we never crash on garbage, and the fingerprint of a
// file with a few corrupted lines should still match the fingerprint
// of the same file without them, because the schema-shape data we
// care about is in the lines that did parse.
func collectFingerprintInputs(r io.Reader) ([]steps.FingerprintInput, bool, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), scannerBufferMax)

	var inputs []steps.FingerprintInput
	parseable := false
	for scanner.Scan() && len(inputs) < maxFingerprintRecords {
		// We decode each line into a map[string]json.RawMessage
		// so we can read the type field and the set of top-level
		// keys without committing to a struct shape. The
		// json.RawMessage type tells the decoder to leave each
		// value as raw bytes, which is enough for what we do
		// next: read the type field and check which top-level
		// keys are present.
		var record map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		parseable = true
		var t string
		if raw, ok := record["type"]; ok {
			_ = json.Unmarshal(raw, &t)
		}
		keys := make([]string, 0, len(record))
		for k := range record {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		inputs = append(inputs, steps.FingerprintInput{Type: t, Keys: keys})
	}
	if err := scanner.Err(); err != nil {
		return nil, parseable, err
	}
	return inputs, parseable, nil
}
