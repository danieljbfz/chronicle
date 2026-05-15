package steps

import "testing"

// TestFingerprint_stableAcrossOrder is the headline property test:
// reordering the inputs must not change the fingerprint. Without this
// guarantee, the same on-disk file would produce different
// fingerprints depending on which record the scanner happened to read
// first, and the lookup table that maps fingerprints to known
// versions would be useless.
func TestFingerprint_stableAcrossOrder(t *testing.T) {
	a := []FingerprintInput{
		{Type: "user", Keys: []string{"uuid", "timestamp", "message"}},
		{Type: "assistant", Keys: []string{"uuid", "timestamp", "message"}},
	}
	b := []FingerprintInput{
		{Type: "assistant", Keys: []string{"message", "uuid", "timestamp"}},
		{Type: "user", Keys: []string{"timestamp", "message", "uuid"}},
	}
	if Fingerprint(a) != Fingerprint(b) {
		t.Errorf("fingerprint changed with reorder: %q vs %q", Fingerprint(a), Fingerprint(b))
	}
}

// TestFingerprint_differentSchemasDiffer is the other half of the
// stability story: when the schema actually does change (a new key
// appears, for example), the fingerprint must change too. Otherwise
// the lookup table cannot tell the new schema apart from the old one
// and chronicle would happily render new data as if it were old.
func TestFingerprint_differentSchemasDiffer(t *testing.T) {
	a := []FingerprintInput{{Type: "user", Keys: []string{"uuid", "message"}}}
	b := []FingerprintInput{{Type: "user", Keys: []string{"uuid", "message", "new_field"}}}
	if Fingerprint(a) == Fingerprint(b) {
		t.Error("adding a key must change the fingerprint")
	}
}

// TestFingerprint_deduplicates confirms that feeding the fingerprinter
// the same tuple repeatedly produces the same hash as feeding it once.
// This matters because real session files contain hundreds of
// duplicate-shape records, and the cost-saving deduplication has to
// stay invisible at the boundary.
func TestFingerprint_deduplicates(t *testing.T) {
	a := []FingerprintInput{{Type: "user", Keys: []string{"uuid"}}}
	b := []FingerprintInput{
		{Type: "user", Keys: []string{"uuid"}},
		{Type: "user", Keys: []string{"uuid"}},
		{Type: "user", Keys: []string{"uuid"}},
	}
	if Fingerprint(a) != Fingerprint(b) {
		t.Error("duplicate tuples should not change the fingerprint")
	}
}

// TestFingerprint_isShortHex verifies the format of the returned
// string. Twelve lowercase hex characters is what the rest of
// chronicle expects, and downstream code (the doctor view, the
// format-report writer) is going to display these strings to humans.
// A wrong format here would break the user-facing surface.
func TestFingerprint_isShortHex(t *testing.T) {
	fp := Fingerprint([]FingerprintInput{{Type: "user", Keys: []string{"x"}}})
	if len(fp) != 12 {
		t.Errorf("fingerprint length = %d, want 12", len(fp))
	}
	for _, r := range fp {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("non-hex char in fingerprint: %q", fp)
			break
		}
	}
}
