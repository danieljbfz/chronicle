package steps

import (
	"encoding/base64"
	"io"
)

// OSC52Sequence returns the terminal escape sequence that copies
// the given text into the system clipboard. The escape sequence is
// part of a terminal protocol called OSC 52, supported by modern
// terminals like iTerm2, kitty, WezTerm, Alacritty, and recent
// versions of xterm and gnome-terminal.
//
// The big win of OSC 52 is that it works over SSH. The escape
// bytes travel as part of the terminal stream, so it is the local
// terminal (the one connected to the user's clipboard) that
// receives them and performs the copy. The remote machine never
// touches anything clipboard-related.
//
// The sequence itself is made of four parts.
//
//  1. ESC, the byte that opens an Operating System Command.
//  2. The literal "52;c;", which says "this is a clipboard write
//     to the system selection". The "c" picks the system clipboard.
//     X11 has two other clipboards called primary and secondary,
//     but most macOS users have no use for either of them.
//  3. The text to copy, encoded in base64. The OSC 52 spec requires
//     base64, and using it also keeps multi-byte or binary text
//     intact through the terminal stream.
//  4. BEL, the byte that closes the sequence.
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
// the bytes still get written. Whether they reach a terminal that
// can interpret them is up to whatever sits at the end of the pipe.
func CopyOSC52(w io.Writer, text string) error {
	_, err := io.WriteString(w, OSC52Sequence(text))
	return err
}
