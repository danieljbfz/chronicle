// Package steps holds pure transforms over the contracts types. No file
// I/O, no time, no environment. Steps are the test-easiest layer of the
// system because every input is explicit and every output is deterministic.
//
// In hexagonal-architecture terms, steps live one layer above contracts:
// they depend only on the domain types and never on adapters or
// composition. The `composition` layer is the one that calls steps with
// real data; tests call steps directly with fixture data.
package steps

// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. `crypto/sha256` AND HASHING. `sha256.Sum256(bytes)` produces a
//    32-byte hash. We hex-encode it to make it printable. The hash is
//    cryptographically strong (collisions are infeasible in practice),
//    which is overkill for schema fingerprints but free — the standard
//    library is fast.
//
// 2. MAPS AS SETS. Go has no built-in set type, but the idiom is
//        m := make(map[KeyType]struct{}, capacityHint)
//        m[key] = struct{}{}
//    The empty struct `struct{}` occupies zero bytes, so this is the
//    cheapest way to ask "have I seen this key before?" The comma-ok
//    form `_, ok := m[key]` returns ok=false when the key is absent.
//
// 3. SLICE TRICKS. `append([]string(nil), in.Keys...)` is the standard
//    "copy this slice" pattern. The `...` after a slice is the *spread*
//    operator: it passes each element of `in.Keys` as a separate argument
//    to append. The leading `[]string(nil)` is an empty nil slice as the
//    destination; appending to nil works fine in Go (appending allocates
//    a new backing array).

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// FingerprintInput is one observed record from a storage stream. The
// Type is the discriminator the adapter found (e.g. "user", "assistant",
// "file-history-snapshot"); Keys are the JSON top-level keys of that
// record.
type FingerprintInput struct {
	Type string
	Keys []string
}

// Fingerprint computes a short, stable hex hash describing the schema
// shape of a session file. Two files with the same set of (record type,
// key set) pairs produce the same fingerprint, so adapters can map
// fingerprints to known versions without parsing every record.
//
// Adapters cap their input at the first N records (typically 200) so the
// fingerprint reflects the variety in the file, not its length.
func Fingerprint(inputs []FingerprintInput) string {
	// Step 1: deduplicate (Type, sorted Keys) tuples. We need uniqueness
	// because a single session has hundreds of "user" records with the
	// same key set — we want to count that as one tuple, not many.
	seen := make(map[string]struct{}, len(inputs))
	var tuples []string
	for _, in := range inputs {
		keys := append([]string(nil), in.Keys...)
		sort.Strings(keys)
		tuple := in.Type + "|" + strings.Join(keys, ",")
		if _, ok := seen[tuple]; ok {
			continue
		}
		seen[tuple] = struct{}{}
		tuples = append(tuples, tuple)
	}

	// Step 2: sort the tuple set so input order does not change the hash.
	sort.Strings(tuples)

	// Step 3: hash the joined tuples and return the first 12 hex chars.
	// 12 chars = 48 bits = collision-safe at our scale (a few thousand
	// fingerprints per user, ever).
	sum := sha256.Sum256([]byte(strings.Join(tuples, "\n")))
	return hex.EncodeToString(sum[:])[:12]
}
