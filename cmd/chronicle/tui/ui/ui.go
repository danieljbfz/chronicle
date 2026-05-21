// Package ui is the shared component layer the TUI screens
// compose their chrome from. Every reusable presentation
// piece chronicle has built that the upstream bubbles
// components do not already provide lives here, with one
// canonical implementation per piece.
//
// Today the package holds the Spinner that the
// loading-capable screens use, with its built-in elapsed-time
// counter that the upstream bubbles spinner does not expose.
// As later phases land the doctor, trash, and memory screens,
// the components they would otherwise hand-roll (confirmation
// modals for the destructive actions, status banners for the
// async fetches, anything else that turns out to repeat
// across screens) land here too. Drift between screens is the
// bug this package exists to prevent.
//
// When a piece of chrome already has a canonical
// implementation upstream — the help row that the bubbles
// help component renders, the divider that lipgloss can draw,
// the table grid that lipgloss/v2/table builds — the screen
// reaches for that upstream component directly rather than
// wrapping it here. Wrapping a complete upstream component is
// the indirection this package does not need.
//
// The package depends only on the chronicle TUI's theme
// package plus the bubbles components it composes. It does
// not depend on the top-level tui package or on any screen
// package, so every screen can import it without creating a
// cycle.
package ui
