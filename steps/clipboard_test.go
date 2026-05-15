package steps

// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. `bytes.Buffer` AS AN io.Writer. `bytes.Buffer` is a growable
//    in-memory byte buffer. It satisfies `io.Writer` (it has a `Write`
//    method), so we can pass `&buf` anywhere an io.Writer is expected
//    and then read back what was written via `buf.String()` or
//    `buf.Bytes()`. This is the standard "fake stdout" technique in
//    Go tests, similar to passing an `io.StringIO()` to a function in
//    Python that takes a file-like object.
//
// 2. SUBTESTS via `t.Run(name, func(t *testing.T) { ... })`. Equivalent
//    to `pytest`'s `@pytest.mark.parametrize` or its own test functions:
//    each subtest gets its own pass/fail line in the output, and you
//    can run a single subtest with `go test -run TestX/subname`.

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

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

func TestCopyOSC52_writesToWriter(t *testing.T) {
	var buf bytes.Buffer
	if err := CopyOSC52(&buf, "abc"); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Error("CopyOSC52 wrote nothing")
	}
}
