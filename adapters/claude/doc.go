// Package claude implements the Provider contract for Claude Code's
// on-disk storage at ~/.claude.
//
// -----------------------------------------------------------------------
// What lives in ~/.claude
// -----------------------------------------------------------------------
//
//	projects/<encoded-cwd>/<sessionId>.jsonl    one file per session
//	file-history/<sessionId>/...                versioned file backups
//	tasks/<sessionId>/...                       per-session task state
//	session-env/<sessionId>                     captured env
//	sessions/<sessionId>.json                   small metadata
//	history.jsonl                               global prompt history
//
// Each session JSONL is a newline-delimited stream of typed records.
// JSONL just means "one JSON object per line" — easy to append to and
// to read incrementally. Python equivalent: `for line in open(path):
// obj = json.loads(line)`.
//
// The parser folds the records into a parent-pointer tree via parentUuid
// and produces a normalized contracts.Conversation. The tree shape lets a
// single ".jsonl" file represent a resumed session with multiple branches —
// useful for the future "show me the tree" feature, ignored for v1's flat
// rendering.
//
// -----------------------------------------------------------------------
// What this package implements right now
// -----------------------------------------------------------------------
//
// Plan A is read-only. Detect, ListProjects, ListSessions, and
// ReadSession do real work; PlanDelete and PlanOrphanScan return
// ErrNotImplemented and Plan C wires them up. That split keeps the early
// commits safe — there is no code path that can delete a file until we
// have built the cascade-delete map and the trash subsystem.
//
// -----------------------------------------------------------------------
// The doc.go convention
// -----------------------------------------------------------------------
//
// Go tools surface the comment immediately above `package <name>` in any
// file as the package's documentation. When the comment is long, the
// convention is to put it in a dedicated file named doc.go that contains
// nothing else. That keeps the comment easy to find and avoids tying
// the documentation lifetime to any one feature file. Python's closest
// analog is the module-level docstring at the top of __init__.py.
package claude
