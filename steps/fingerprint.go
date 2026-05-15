// Package steps holds pure transforms over the contracts types. No file I/O,
// no time, no environment. Steps are the test-easiest layer of the system.
package steps

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// FingerprintInput is one observed record from a storage stream. The Type
// is the discriminator the adapter found (e.g. "user", "assistant",
// "file-history-snapshot"); Keys are the JSON top-level keys of that record.
type FingerprintInput struct {
	Type string
	Keys []string
}

// Fingerprint computes a short, stable hex hash describing the schema shape
// of a session file. Two files with the same set of (record type, key set)
// pairs produce the same fingerprint, so adapters can map fingerprints to
// known versions without parsing every record.
//
// Adapters cap their input at the first N records (typically 200) so the
// fingerprint reflects the variety in the file, not its length.
func Fingerprint(inputs []FingerprintInput) string {
	// Step 1: deduplicate (Type, sorted Keys) tuples.
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
	sum := sha256.Sum256([]byte(strings.Join(tuples, "\n")))
	return hex.EncodeToString(sum[:])[:12]
}
