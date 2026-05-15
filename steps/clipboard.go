package steps

// -----------------------------------------------------------------------
// What this file does
// -----------------------------------------------------------------------
//
// OSC 52 is the terminal escape sequence that means "load this text into
// the system clipboard." Modern terminals (iTerm2, kitty, WezTerm,
// Alacritty, recent xterm and gnome-terminal) honour it. The headline
// feature is that it works transparently over SSH: the bytes travel as
// part of the terminal stream, so the *local* terminal — the one with
// access to your clipboard — is the one that interprets them and copies.
//
// Python equivalents like `pyperclip` shell out to `pbcopy`/`xclip`,
// which only see the *remote* clipboard when run over SSH. OSC 52 sidesteps
// that whole problem because the escape sequence is just bytes; the
// terminal does the clipboard dance for us.
//
// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. `encoding/base64`. The standard library's base64 codec. The OSC 52
//    spec wants the payload base64-encoded so binary or multi-byte text
//    survives the terminal stream intact. `base64.StdEncoding.EncodeToString`
//    takes `[]byte` and returns the encoded string.
//
// 2. `io.Writer` AS A FUNCTION PARAMETER. The CopyOSC52 function takes
//    `w io.Writer` instead of writing directly to stdout. That single
//    line of polymorphism means tests can pass a `bytes.Buffer` and
//    inspect what was written, the CLI passes `os.Stdout`, and a future
//    feature could pass an HTTP response writer — all without changing
//    this code. This pattern is everywhere in idiomatic Go.
//
// 3. THE BACKTICK STRING vs THE DOUBLE-QUOTED STRING. Double quotes
//    interpret escape sequences (`"\n"` is one newline byte). Backticks
//    are raw strings — `` `\n` `` is two characters, a backslash and an n.
//    Below we use double quotes so `\x1b` (ESC) and `\x07` (BEL) are
//    actual control bytes, not the literal text.

import (
	"encoding/base64"
	"io"
)

// OSC52Sequence returns the terminal escape sequence that loads `text`
// into the system clipboard via OSC 52. The "c" selector targets the
// system clipboard (versus the X primary or secondary selections, which
// most macOS users do not care about).
//
// Anatomy of the returned string:
//
//	\x1b]52;c;<base64-of-text>\x07
//	  │  │ │ │     │             │
//	  │  │ │ │     │             └── BEL terminator
//	  │  │ │ │     └────────────── payload
//	  │  │ │ └──────────────────── selector ("c" = clipboard)
//	  │  │ └────────────────────── OSC 52 command number
//	  │  └──────────────────────── OSC introducer
//	  └─────────────────────────── ESC
func OSC52Sequence(text string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	return "\x1b]52;c;" + encoded + "\x07"
}

// CopyOSC52 writes the OSC 52 sequence for `text` to `w`. The CLI passes
// os.Stdout from the calling command — terminals interpret the sequence
// as it streams, so there is no need to flush or call any clipboard API.
//
// We return the writer's error verbatim. If the user is piping our
// output somewhere that does not understand OSC 52 (a file, another
// program), the bytes still get written; whether the *user's* terminal
// later interprets them is up to that terminal.
func CopyOSC52(w io.Writer, text string) error {
	_, err := io.WriteString(w, OSC52Sequence(text))
	return err
}
