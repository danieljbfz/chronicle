// Package keys carries the keyboard bindings the chronicle TUI
// shares across every screen. The bindings are declarative values
// of type key.Binding, which gives each binding a list of literal
// keys and a help-text entry the help bar can render. The
// declarative model is what bubbles/v2/key, bubbles/v2/list, and
// bubbles/v2/viewport all expect, so the same KeyMap value flows
// straight into the components without translation.
//
// The default bindings follow the Vim conventions where the Vim
// equivalent is obvious, and add arrow keys and Enter as fallbacks
// so a user who does not live inside Vim can still drive every
// screen. The bindings are deliberately shared across screens so
// "j" and "k" mean "down" and "up" everywhere, "enter" means
// "open the highlighted item" everywhere, and "?" means "show
// help" everywhere. Per-screen bindings live next to their
// screens, not in this file.
package keys

import "charm.land/bubbles/v2/key"

// KeyMap holds the cross-screen bindings the top-level model and
// every per-screen model rely on. The bindings are values, not
// pointers, because key.Binding is itself a value type and the
// help-text rendering uses the binding's own methods rather than
// reaching into private state.
type KeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Top     key.Binding
	Bottom  key.Binding
	Enter   key.Binding
	Back    key.Binding
	Filter  key.Binding
	Refresh key.Binding
	Help    key.Binding
	Quit    key.Binding
}

// Default returns the keyboard bindings every chronicle screen
// inherits. A screen that wants extra bindings should declare a
// second KeyMap next to its own model rather than mutating this
// one — that way the screen owns its own help text and the shared
// bindings stay readable.
func Default() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Top: key.NewBinding(
			key.WithKeys("g", "home"),
			key.WithHelp("g", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G", "bottom"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "open"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// ShortHelp returns the most-used bindings in the order the help
// bar should render them. The bubbles/v2 help component calls this
// when the bar is in its compact one-line form.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Filter, k.Help, k.Quit}
}

// ViewportShortHelp returns the bindings every viewport-driven
// screen advertises in its help line beyond the global short
// help. The bubbles viewport accepts u and d for half-page
// jumps and g/G for top and bottom, and the same hints apply
// across the transcript reader, the stats summary, and any
// later screen built on the same component. Defining the set
// here keeps the three screens from holding three parallel
// slices that could drift apart.
func (k KeyMap) ViewportShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("u", "d"), key.WithHelp("u/d", "half page")),
		key.NewBinding(key.WithKeys("g", "G"), key.WithHelp("g/G", "top/bottom")),
	}
}

// FullHelp returns the same bindings grouped into columns, for the
// expanded view a user reaches by pressing "?". Each inner slice is
// one column. The grouping is by purpose — navigation in the first
// column, actions in the second, meta-controls in the third — so
// the layout reads predictably regardless of terminal width.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.Enter, k.Filter, k.Refresh, k.Back},
		{k.Help, k.Quit},
	}
}
