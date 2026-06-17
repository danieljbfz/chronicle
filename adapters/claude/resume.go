package claude

import (
	"encoding/json"
	"io/fs"

	"github.com/danieljbfz/chronicle/contracts"
)

// claudeExecutable is the name chronicle uses for the
// upstream CLI when constructing the resume plan. We do not
// look up the absolute path here. The CLI layer lets
// exec.Command resolve it through PATH at exec time, the
// same way the user would when typing the command directly.
// That keeps chronicle's behaviour predictable across
// installs that put the binary in different places.
const claudeExecutable = "claude"

// ResumeCommand finds the session on disk, reads the cwd
// the upstream tool recorded inside the session JSONL, and
// returns the command the CLI should exec. The function
// returns a wrapped fs.ErrNotExist when the session
// identifier is not found in this provider, which lets
// composition test for it with errors.Is.
//
// The cwd comes from the JSONL records rather than from
// decoding the encoded folder name. The folder-name
// encoding is lossy: a real path like
// /Users/x/work/claude-history and the synthetic path
// /Users/x/work/claude/history both encode to the same
// folder. Reading the cwd field that Claude itself wrote
// into the session is the only way to recover the
// authoritative value. The folder-name decode stays as a
// last-resort fallback for sessions that, for whatever
// reason, never recorded a cwd.
func (p *Provider) ResumeCommand(root fs.FS, id contracts.SessionID) (contracts.ResumePlan, error) {
	sessionFile, err := locateSessionFile(root, id)
	if err != nil {
		return contracts.ResumePlan{}, newError("resume command", string(id), err)
	}

	cwd, err := readSessionCwd(root, sessionFile)
	if err != nil {
		return contracts.ResumePlan{}, newError("resume command", sessionFile, err)
	}
	if cwd == "" {
		// Fallback: derive the cwd from the encoded folder
		// name. This is lossy (see the doc on
		// decodeProjectPath), but a degraded answer is
		// strictly better than failing the whole resume.
		// Sessions written by current Claude versions
		// always carry a cwd, so this branch only fires for
		// very old sessions or hand-edited fixtures.
		cwd = decodeProjectPath(projectFolderFromSessionPath(sessionFile))
	}

	return contracts.ResumePlan{
		Command:    []string{claudeExecutable, "--resume", string(id)},
		WorkingDir: cwd,
	}, nil
}

// readSessionCwd streams the JSONL session file just far
// enough to find the first record that carries a non-empty
// cwd field, then stops. We avoid invoking the full session
// parser because that loads every message into memory, and
// the cwd lands within the first few records (usually the
// third, after the leaf-uuid and permission-mode headers
// that Claude writes at the top of the file).
//
// The function returns the empty string and a nil error
// when the scan ends without a cwd, whether the file parsed
// cleanly to its end or the stream became unreadable partway
// through. That empty result is the sentinel the caller uses
// to decide whether to fall back to the lossy folder-name
// decode.
func readSessionCwd(root fs.FS, sessionFile string) (string, error) {
	f, err := root.Open(sessionFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	for {
		var record struct {
			Cwd string `json:"cwd"`
		}
		if err := dec.Decode(&record); err != nil {
			// A clean EOF means the whole file was read without
			// a cwd. Any other error means the stream can no
			// longer be parsed: json.Decoder does not recover
			// from a syntax error or a value truncated mid-write
			// (the shape a session file takes when it is read
			// while Claude is still appending), and every later
			// Decode would return that same error. Either way the
			// scan is over, and the empty result sends the caller
			// to the folder-name fallback.
			return "", nil
		}
		if record.Cwd != "" {
			return record.Cwd, nil
		}
	}
}

// Compile-time check: *Provider satisfies the optional
// Resumable capability. If a future refactor changes the
// interface signature, this line surfaces the mismatch at
// build time instead of letting the runtime type assertion
// composition uses silently return ok=false and drop the
// capability without a word.
var _ contracts.Resumable = (*Provider)(nil)
