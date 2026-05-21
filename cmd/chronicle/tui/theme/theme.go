// Package theme carries the lipgloss styles every chronicle screen
// shares. The defaults follow the terminal's own palette so the
// TUI blends into a user's existing colour scheme, and an opt-in
// dark variant is available for users whose terminal does not have
// one already. The variant is chosen at construction time and the
// styles inside the returned Theme are immutable — a screen that
// wants a one-off style derives it from the shared value rather
// than mutating it.
package theme

import "charm.land/lipgloss/v2"

// Separator is the bullet character the TUI uses everywhere two
// peer items sit beside each other on one line — tabs in the
// section strip, fields in the transcript subtitle, key-and-
// description pairs in the help row that the bubbles help
// component renders. Using one canonical separator across every
// surface keeps the chrome visually coherent. The bullet
// (U+2022) matches the bubbles help component's default
// ShortSeparator, so the help row at the bottom of every screen
// reads the same as the strip at the top.
const Separator = " • "

// HierarchySeparator is the chevron the TUI uses for parent →
// child relationships in a breadcrumb. The shape distinguishes
// hierarchical navigation from the flat peer separation
// Separator carries, so a reader sees the difference at a
// glance between "sessions and stats are peer sections" and
// "sessions is the parent of this transcript".
const HierarchySeparator = " › "

// Variant names a colour scheme. Today the two variants are the
// terminal's native palette and a hand-tuned dark scheme. A future
// "light" variant can land here without changing call sites.
type Variant int

const (
	// VariantTerminal is the default. Foreground and background
	// colours follow whatever the terminal emulator picked, so the
	// TUI inherits the user's existing solarized, gruvbox, or
	// Nord palette without any extra configuration.
	VariantTerminal Variant = iota

	// VariantDark is the opt-in scheme for users whose terminal
	// does not ship a colour palette, or who want the chronicle
	// surface to feel consistent across different machines.
	VariantDark
)

// Theme is the bundle of styles every screen draws from. Each
// field is a lipgloss.Style value, ready to be passed to a bubbles
// component's With… setter or to a hand-rendered string. The
// fields are grouped by role rather than by colour, because the
// reader almost always knows the role of the text they are
// styling and only rarely knows the exact colour they want.
type Theme struct {
	// Variant is the scheme the theme was built from. Callers can
	// branch on it for rare cases where a style needs to swap
	// based on the active scheme.
	Variant Variant

	// Title is the screen-level heading, rendered at the top of
	// every screen.
	Title lipgloss.Style

	// Subtitle is the secondary heading underneath Title, used for
	// counts, dates, or "no data" notes.
	Subtitle lipgloss.Style

	// Muted is the style for text that should fade into the
	// background — timestamps, filenames, paths, anything the
	// reader scans rather than reads.
	Muted lipgloss.Style

	// Highlight is the style for the currently focused row or
	// item. The bubbles list component already paints a focused
	// row, but lower-level renderings (the doctor cards, the
	// stats table) need their own focus paint.
	Highlight lipgloss.Style

	// Accent is the style for action verbs and key hints in the
	// help bar.
	Accent lipgloss.Style

	// Error is the style for error banners and warnings.
	Error lipgloss.Style

	// HelpKey and HelpDesc style the two halves of a help-bar
	// entry: "↑/k" then "up", "?" then "help".
	HelpKey  lipgloss.Style
	HelpDesc lipgloss.Style

	// Border is the style for any boxed region — the doctor cards,
	// modal dialogs, the help overlay.
	Border lipgloss.Style
}

// New returns the theme for the requested variant. The returned
// Theme is safe to share across screens.
func New(v Variant) Theme {
	switch v {
	case VariantDark:
		return darkTheme()
	default:
		return terminalTheme()
	}
}

func terminalTheme() Theme {
	return Theme{
		Variant:   VariantTerminal,
		Title:     lipgloss.NewStyle().Bold(true),
		Subtitle:  lipgloss.NewStyle().Faint(true),
		Muted:     lipgloss.NewStyle().Faint(true),
		Highlight: lipgloss.NewStyle().Reverse(true),
		Accent:    lipgloss.NewStyle().Bold(true),
		Error:     lipgloss.NewStyle().Bold(true),
		HelpKey:   lipgloss.NewStyle().Bold(true),
		HelpDesc:  lipgloss.NewStyle().Faint(true),
		Border:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()),
	}
}

func darkTheme() Theme {
	// The dark variant fixes colours rather than inheriting from
	// the terminal. The palette is deliberately small — one
	// foreground, one muted foreground, one accent, one error —
	// so the screen reads as composed rather than as a swatch
	// reel. The hex values match the chronicle logo's two-tone
	// look so the TUI feels of-a-piece with the project's docs.
	const (
		foreground    = "#E6E6E6"
		mutedColor    = "#8A8A8A"
		accentColor   = "#7AA2F7"
		highlightBack = "#1F2335"
		errorColor    = "#F7768E"
		borderColor   = "#3B4261"
	)

	base := lipgloss.NewStyle().Foreground(lipgloss.Color(foreground))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(mutedColor))

	return Theme{
		Variant:   VariantDark,
		Title:     base.Bold(true),
		Subtitle:  muted,
		Muted:     muted,
		Highlight: base.Background(lipgloss.Color(highlightBack)).Bold(true),
		Accent:    base.Foreground(lipgloss.Color(accentColor)).Bold(true),
		Error:     base.Foreground(lipgloss.Color(errorColor)).Bold(true),
		HelpKey:   base.Foreground(lipgloss.Color(accentColor)).Bold(true),
		HelpDesc:  muted,
		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(borderColor)),
	}
}
