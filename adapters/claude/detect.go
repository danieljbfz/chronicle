package claude

// -----------------------------------------------------------------------
// What this file does
// -----------------------------------------------------------------------
//
// detect.go answers the question "which version of Claude Code's storage
// are we looking at?" without parsing the entire file. It scans the first
// ~200 records, collects the (record-type, key-set) tuples it sees, and
// hashes them into a 12-character fingerprint. If the fingerprint matches
// one in `knownFingerprints`, we know the version; otherwise we return
// Version = "unknown" and the rest of the system degrades gracefully.
//
// Detection runs once per chronicle process, cached by the Provider type.
// Reading 200 lines of one session file is essentially free.
//
// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. `bufio.Scanner`. The standard idiom for reading a file line-by-line.
//    Python's `for line in open(path):` returns each line including its
//    trailing newline; Go's Scanner strips it for you. The default buffer
//    is 64 KB per line — a single 22 MB Claude session has lines longer
//    than that, so we explicitly raise the buffer with `scanner.Buffer`.
//
// 2. `io/fs` and `fs.FS`. The testable-filesystem interface. A function
//    that takes `root fs.FS` cannot tell whether it is reading the user's
//    real ~/.claude or an in-memory `fstest.MapFS` set up by a test. This
//    is the pattern that makes adapter tests cheap and reliable. Compare
//    Python's `pathlib.Path` (always a real path) versus passing a
//    file-like object — fs.FS is the latter, lifted to whole filesystems.
//
// 3. NAMED RETURN VALUES. The signature
//        func collectFingerprintInputs(r io.Reader) (inputs []steps.FingerprintInput, parseable bool, err error)
//    names each return slot. Inside the function, the names act as
//    pre-declared variables. A bare `return` at the end returns whatever
//    those variables currently hold. Useful when a function returns three
//    things and the names communicate intent better than positional
//    `return foo, bar, baz`.
//
// 4. ERROR-WRAPPING via `errors.Is(err, fs.ErrNotExist)`. Same idea as
//    the config loader: ask whether an error is, or wraps, a sentinel.
//    When the user has no projects/ directory yet, that is not an
//    error — Detect should just say "unknown" and let composition skip
//    this provider gracefully.

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

const (
	// adapterName is the string returned by Provider.Name and stamped on
	// every StorageVersion.Adapter this package produces.
	adapterName = "claude"

	// maxFingerprintRecords caps how many records contribute to a
	// fingerprint. The first records carry the structural variety; reading
	// a 22 MB JSONL just to fingerprint it would be wasteful.
	maxFingerprintRecords = 200

	// projectsDir is the subdirectory under the Claude root that holds
	// per-cwd session folders.
	projectsDir = "projects"

	// scannerBufferMax raises bufio.Scanner's per-line buffer ceiling.
	// A single Claude assistant turn with thinking + many tool uses can
	// exceed the default 64 KB. 16 MB is generous and harmless.
	scannerBufferMax = 16 * 1024 * 1024
)

// knownFingerprints maps detected fingerprints to internal version codes.
// New entries land here as the upstream format evolves. When we hit a
// fingerprint not in this map, we return "unknown" — read-only operations
// still work via tolerant parsing.
//
// The map is populated empirically: when chronicle first runs against a
// new Claude Code version, `chronicle doctor` shows the fingerprint we
// observed. We then add it here in a follow-up commit. Plan A's final
// task (Task 20) does this for the version on the contributor's machine.
var knownFingerprints = map[string]string{
	// Empty for now. Task 20 adds the first entry.
}

// detectInDir computes the fingerprint and version for the first session
// file found under the directory tree. It is the building block for the
// Provider.Detect implementation.
//
// "First session file" is fine: every session in this Claude install was
// written by the same Claude Code version, so the fingerprint will agree.
// If we ever need per-session detection, we add it then.
func detectInDir(root fs.FS) (contracts.StorageVersion, error) {
	file, err := firstSessionFile(root)
	if errors.Is(err, fs.ErrNotExist) {
		// No projects directory or no session files inside it. This is
		// not an error — chronicle should still load and the doctor view
		// will show "no Claude data found here yet."
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
		// The file existed but every line failed JSON decoding. That is
		// the one shape that *is* an error per the resilience contract:
		// "the file is not parseable as any known format at all."
		return contracts.StorageVersion{}, errors.New("no parseable JSON records in " + file)
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
			ModelMetadata:   false, // Claude's JSONL does not carry per-turn model id
		},
	}, nil
}

// firstSessionFile walks projects/<*>/<*>.jsonl and returns the path of
// the first one it finds. Returns fs.ErrNotExist when no session file is
// present anywhere under the projects directory.
//
// We use `path` (not `path/filepath`) because the fs.FS interface
// always uses forward slashes, regardless of OS. `path/filepath` is for
// real OS paths — they are different packages by design.
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

// readFingerprintInputs streams a JSONL file and returns up to
// maxFingerprintRecords (type, keys) tuples for the fingerprinter.
// `parseable` is true once any line decoded as JSON.
//
// The function is split out from collectFingerprintInputs so the
// file-handling part stays trivial (open, defer close, hand the reader
// to the parser). collectFingerprintInputs has no I/O at all and is the
// part that matters for tests.
func readFingerprintInputs(root fs.FS, file string) (inputs []steps.FingerprintInput, parseable bool, err error) {
	f, err := root.Open(file)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	return collectFingerprintInputs(f)
}

// collectFingerprintInputs reads JSONL lines from r and produces one
// FingerprintInput per line, skipping any line that fails JSON decoding
// (the resilience contract allows this — we never crash on garbage).
//
// `parseable` becomes true the first time we successfully decode a line.
// If it stays false, the caller treats the file as completely opaque.
func collectFingerprintInputs(r io.Reader) ([]steps.FingerprintInput, bool, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), scannerBufferMax)

	var inputs []steps.FingerprintInput
	parseable := false
	for scanner.Scan() && len(inputs) < maxFingerprintRecords {
		// Decode each line into a generic map[string]json.RawMessage so
		// we can inspect the keys without committing to a struct shape.
		// `json.RawMessage` is the standard library's "leave this alone
		// for now" type — see contracts/block.go.
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
