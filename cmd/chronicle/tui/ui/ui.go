// Package ui is the shared component layer the TUI screens
// compose their chrome from. Every reusable presentation piece
// chronicle's screens need — the help bar at the bottom, the
// loading spinner that announces a fetch is in flight, the
// full-sentence status renderer — lives in this package and is
// rendered through one canonical implementation per piece.
//
// The point is the visual consistency a polished web product
// gets from a design-system layer. A page on Linear or Stripe
// does not hand-render its own button or its own breadcrumb;
// it composes the design system's button and breadcrumb. Each
// chronicle screen does the same here: a screen never builds
// the help line itself, it calls ui.HelpBar with the extra
// bindings it wants to advertise and the renderer takes care
// of the rest. The shared part is the renderer. The content
// the renderer receives is up to each screen.
//
// The package depends only on the chronicle TUI's keys and
// theme packages plus the upstream bubbles components it
// wraps. It does not depend on the top-level tui package or on
// any screen package, so every screen can import it without
// creating a cycle.
package ui
