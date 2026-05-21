package tui

// This file is the home for tea.Msg types that genuinely need to
// be shared between more than two packages — for example, a
// "session selected" message that both the sessions screen and
// the trash screen would emit if they both opened the transcript
// reader. The file is intentionally empty until that need is
// real.
//
// Per-screen intents — including the "user pressed Enter on a
// row, open the transcript reader" message the session list
// emits — live inside the screen's own package. The top-level
// app model imports the screen to use the screen's Model type,
// so it can also reach the screen's own message types without a
// cycle. Adding a shared type here only pays for itself when a
// second screen needs to emit the exact same intent, and that
// has not happened yet.
