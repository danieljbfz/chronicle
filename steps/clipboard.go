package steps

import (
	"encoding/base64"
	"io"
)

// OSC52Sequence returns the terminal escape sequence that loads the
// given text into the system clipboard through the OSC 52 protocol.
// Modern terminals like iTerm2, kitty, WezTerm, Alacritty, and recent
// versions of xterm and gnome-terminal all support OSC 52, and the
// headline benefit is that it works transparently over SSH. The
// escape bytes travel as part of the terminal stream, so the local
// terminal — the one with access to the user's clipboard — is the one
// that interprets them and performs the copy.
//
// The sequence has four parts. ESC opens an Operating System Command,
// the literal "52;c;" identifies it as a clipboard write to the
// system selection (the "c" picks the system clipboard rather than
// X's primary or secondary selection, which most macOS users do not
// have a use for), the base64-encoded payload follows, and the BEL
// byte terminates the sequence. We base64-encode the payload because
// the OSC 52 specification requires it and because base64 keeps
// binary or multi-byte text intact through the terminal stream.
func OSC52Sequence(text string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	return "\x1b]52;c;" + encoded + "\x07"
}

// CopyOSC52 writes the OSC 52 escape sequence for text into w. The
// command-line implementation passes os.Stdout, because terminals
// interpret the sequence as it streams and there is nothing else for
// us to call. We accept io.Writer rather than *os.File so the test
// suite can pass a bytes.Buffer and inspect what was written, and so
// a future caller (a logging wrapper, a shell-out for verification)
// can plug in something else without changing this code.
//
// We return the writer's error verbatim. If the user pipes
// chronicle's output into something that does not understand OSC 52,
// the bytes still get written; whether they reach a terminal that
// can interpret them is up to whatever sits at the end of the pipe.
func CopyOSC52(w io.Writer, text string) error {
	_, err := io.WriteString(w, OSC52Sequence(text))
	return err
}
