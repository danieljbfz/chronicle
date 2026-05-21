package tui

// This file is the home for tea.Msg types that cross screen
// boundaries. When phase 1 introduces the session list, the
// message that says "the user picked this session, open the
// transcript reader" will live here, because both the session
// list screen and the top-level app model need to refer to it.
//
// Per-screen messages — the ones that only one screen ever sees —
// stay inside that screen's own package. The split keeps the
// shared message surface small and easy to audit.
//
// The file is intentionally empty during phase 0. Phase 1 will
// add the first cross-screen message.
