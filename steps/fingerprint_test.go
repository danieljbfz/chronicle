package steps

import "testing"

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

func TestFingerprint_differentSchemasDiffer(t *testing.T) {
	a := []FingerprintInput{{Type: "user", Keys: []string{"uuid", "message"}}}
	b := []FingerprintInput{{Type: "user", Keys: []string{"uuid", "message", "new_field"}}}
	if Fingerprint(a) == Fingerprint(b) {
		t.Error("adding a key must change the fingerprint")
	}
}

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
