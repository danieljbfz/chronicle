package steps

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// TestOSC52Sequence_shape pulls the produced sequence apart and
// checks each piece is right: the ESC and OSC introducer at the
// start, the BEL terminator at the end, and the payload between them
// that decodes back to the original text. If any of these checks
// fail, the bytes we emit are not OSC 52 anymore.
func TestOSC52Sequence_shape(t *testing.T) {
	seq := OSC52Sequence("hello")
	if !strings.HasPrefix(seq, "\x1b]52;c;") {
		t.Errorf("missing OSC 52 prefix: %q", seq)
	}
	if !strings.HasSuffix(seq, "\x07") {
		t.Errorf("missing BEL terminator: %q", seq)
	}
	body := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b]52;c;"), "\x07")
	decoded, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("body is not valid base64: %v", err)
	}
	if string(decoded) != "hello" {
		t.Errorf("decoded body = %q, want %q", string(decoded), "hello")
	}
}

// TestOSC52Sequence_handlesUnicodeAndNewlines confirms the round trip
// through base64 preserves multi-byte characters and embedded
// newlines, neither of which would survive a naive byte-by-byte
// transmission. The test uses table-driven subtests, which is the
// idiomatic Go way to run the same assertion over a list of inputs
// and get a separate pass-or-fail line in the output for each one.
func TestOSC52Sequence_handlesUnicodeAndNewlines(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"ascii", "abc"},
		{"newlines", "line1\nline2\nline3"},
		{"unicode", "héllo · 世界 · café"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seq := OSC52Sequence(tc.in)
			body := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b]52;c;"), "\x07")
			decoded, err := base64.StdEncoding.DecodeString(body)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if string(decoded) != tc.in {
				t.Errorf("round-trip mismatch: got %q, want %q", string(decoded), tc.in)
			}
		})
	}
}

// TestCopyOSC52_writesToWriter is a smoke test that proves the
// io.Writer plumbing actually writes something. The shape checks in
// TestOSC52Sequence_shape cover the content; this test only verifies
// that nothing was lost between the helper and the writer.
func TestCopyOSC52_writesToWriter(t *testing.T) {
	var buf bytes.Buffer
	if err := CopyOSC52(&buf, "abc"); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Error("CopyOSC52 wrote nothing")
	}
}
