// Package steps holds the pure transformations that turn one shape of
// data into another. Nothing in this package opens a file, reads the
// clock, or looks at an environment variable. That makes the steps
// the easiest layer of chronicle to test, because every input is
// explicit and every output is deterministic.
//
// In hexagonal-architecture terms, the steps live one layer above
// contracts. They depend only on the domain types and never on the
// adapters or on composition. The composition layer is the one that
// calls steps with real data, while the tests call them directly with
// fixture data.
package steps

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// FingerprintInput is one observed record from a storage stream, in
// the shape the fingerprinter needs. The Type is the discriminator
// the adapter found, like "user" or "assistant" or
// "file-history-snapshot," and the Keys are the JSON top-level keys
// of that record. Adapters never feed the fingerprinter raw bytes,
// because doing so would tie the fingerprint to whitespace and field
// order. Feeding it the type and the key set instead means a
// reformatted file produces the same fingerprint, which is exactly
// what we want.
type FingerprintInput struct {
	Type string
	Keys []string
}

// Fingerprint computes a short, stable hex hash that describes the
// schema shape of a session file. Two files with the same set of
// (record type, key set) pairs produce the same fingerprint, so the
// adapters can map fingerprints to known versions through a small
// lookup table without parsing every record.
//
// Adapters cap their input at the first couple of hundred records, so
// the fingerprint reflects the variety of shapes in the file rather
// than its length. Reading more would not change the hash, because
// the first records carry every record type the file uses in
// practice.
func Fingerprint(inputs []FingerprintInput) string {
	// Step 1: deduplicate the (Type, sorted Keys) tuples. We need
	// uniqueness because a single session file has hundreds of
	// "user" records that all share the same set of keys, and we
	// want to count that as one distinct shape rather than as many.
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

	// Step 2: sort the deduplicated tuple set so that the order in
	// which the adapter happened to encounter the records does not
	// influence the final hash.
	sort.Strings(tuples)

	// Step 3: hash the joined tuples and return the first twelve hex
	// characters. Twelve hex characters is forty-eight bits, which is
	// collision-safe at our scale. We will never see more than a few
	// thousand fingerprints across all the chronicle installs that
	// will ever exist.
	sum := sha256.Sum256([]byte(strings.Join(tuples, "\n")))
	return hex.EncodeToString(sum[:])[:12]
}
