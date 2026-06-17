package contracts

import "time"

// SessionRef is a parse-free handle to one session on disk. An adapter
// produces one per session by reading the directory and stat data only,
// never the session's content, so enumerating a large install stays
// cheap. The fields a listing summary needs that do not require parsing
// — the size and the identifiers — ride on the ref, and Locator is the
// adapter's own path to the session, opaque to every caller above the
// adapter and used only to parse the session on a cache miss.
type SessionRef struct {
	ID        SessionID
	Project   ProjectID
	SizeBytes int64
	ModTime   time.Time
	Locator   string
}
