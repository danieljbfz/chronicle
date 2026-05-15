// Package claude is the chronicle adapter for Claude Code's on-disk
// storage at ~/.claude. The job of an adapter is to translate the
// messy files one specific tool writes to disk into the clean,
// shared types defined in the contracts package, so the rest of
// chronicle can render and operate on conversations without ever
// having to know how Claude Code's storage is laid out.
//
// The directory layout under ~/.claude is roughly the following:
//
//	projects/<encoded-cwd>/<sessionId>.jsonl    one file per session
//	file-history/<sessionId>/...                versioned file backups
//	tasks/<sessionId>/...                       per-session task state
//	session-env/<sessionId>                     captured env
//	sessions/<sessionId>.json                   small metadata
//	history.jsonl                               global prompt history
//
// Each session file is JSONL, which means one JSON object per line.
// JSONL is a very common log format because it is trivially
// appendable and trivially streamable: a writer adds lines at the end
// and a reader consumes them one at a time without ever having to
// parse the whole file. Claude Code uses it for exactly that reason.
//
// The parser folds the records into a parent-pointer tree by way of
// each record's parentUuid field. The tree shape lets a single .jsonl
// file represent a session that was resumed and then branched, which
// the user interface will be able to render as a tree in a future
// plan. For the version-one chronicle, the tree gets flattened to a
// chronological list, but the data is already there for later.
//
// The package today is read-only. *Provider implements
// contracts.Provider but not contracts.Cleaner. The cascade-aware
// cleanup work will add the Cleaner methods once the trash
// subsystem is ready. The split is deliberate: the destructive
// code paths do not exist yet, so nothing chronicle does today
// can accidentally delete anything from a Claude session.
package claude
