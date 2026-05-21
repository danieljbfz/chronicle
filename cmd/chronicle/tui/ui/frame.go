package ui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
)

// Frame is the single rendering rule every chronicle TUI screen
// composes through. The point is that a reader looking at the
// rendered TUI cannot tell which screen drew which row of
// chrome: every loading state, every empty state, every error
// state, and every help-row footer reads the same because there
// is exactly one implementation. A screen's only job is to
// supply its content and its status; the frame draws the rest.
//
// The frame fills a rectangle of width × height with three
// stacked regions. The body region sits at the top and is
// padded down with blank rows so the footer stays anchored to
// the bottom of the rectangle regardless of how short the body
// happens to be. An optional status row sits between the body
// and the footer in a muted register, so a screen can report
// what it is doing without competing with the body for the
// reader's eye. The footer carries a short, single-line help
// row of the bindings most useful in the current screen —
// every binding the screen offers also lives in the full-help
// overlay the app opens when the user presses `?`, so the
// footer optimises for in-context discovery without cluttering
// the layout.
//
// The frame does not own the screen's keybindings, its loading
// commands, or the global help overlay. Those still live in
// each screen and in the app model, because each one is the
// right home for the concern. The frame is a renderer, not a
// model.
type Frame struct {
	theme theme.Theme
	keys  keys.KeyMap
	help  help.Model
}

// NewFrame returns a Frame configured for the chronicle theme.
// The frame holds no per-render state, so a screen can reuse
// one Frame value across every View call. The help component
// the frame composes is preconfigured for the theme's help-row
// styles, so the help row reads identically across every
// screen the frame renders.
func NewFrame(t theme.Theme, k keys.KeyMap) Frame {
	h := help.New()
	h.Styles.ShortKey = t.HelpKey
	h.Styles.ShortDesc = t.HelpDesc
	h.Styles.ShortSeparator = t.Muted
	h.Styles.FullKey = t.HelpKey
	h.Styles.FullDesc = t.HelpDesc
	h.Styles.FullSeparator = t.Muted
	h.Styles.Ellipsis = t.Muted
	return Frame{theme: t, keys: k, help: h}
}

// State is the screen's request to the frame: what to draw in
// the body region for this render. The four constructors below
// — Loading, Empty, Error, Ready — produce the four possible
// states. A screen never reaches into the State directly; it
// calls one of the constructors with the data it wants the
// frame to surface.
type State struct {
	kind    stateKind
	content string
	spinner Spinner

	headline string
	detail   string
	err      error
}

type stateKind int

const (
	stateReady stateKind = iota
	stateLoading
	stateEmpty
	stateError
)

// Loading is the state a screen returns while an asynchronous
// fetch is in flight. The spinner is the live-elapsed-time
// indicator the screen holds for the duration of the load; the
// frame renders its View row at the top of the body region.
// Every screen's loading row reads the same shape — animated
// glyph, screen-provided message, parenthesised elapsed time
// — so the user sees one consistent loading experience.
func Loading(s Spinner) State {
	return State{kind: stateLoading, spinner: s}
}

// Empty is the state a screen returns when the fetch succeeded
// but produced no rows. The headline is a one-sentence summary
// of the absence ("No sessions found across any detected
// provider."), and detail is the next-step prose the screen
// wants the user to read ("Run `chronicle doctor` to check
// whether the provider roots are reachable.").
func Empty(headline, detail string) State {
	return State{kind: stateEmpty, headline: headline, detail: detail}
}

// Error is the state a screen returns when the fetch failed.
// The frame paints the error in the theme's error style with
// the underlying error message quoted verbatim, then renders
// the detail prose underneath so the user knows the next
// action.
func Error(err error, detail string) State {
	return State{kind: stateError, err: err, detail: detail}
}

// Ready is the state a screen returns when its content is in
// hand and ready to render. The string is the screen's body
// content, already sized to the body region's expected width.
// The screen owns the layout inside that content; the frame
// only takes care of placing it inside the body region and
// keeping the footer anchored beneath it.
func Ready(content string) State {
	return State{kind: stateReady, content: content}
}

// Render lays the screen out. The width and height arguments
// are the dimensions of the rectangle the frame fills — the
// same dimensions the screen's model received through the
// runtime's last WindowSizeMsg, minus any chrome the app
// draws above the screen. Footer is the slice of bindings the
// screen wants advertised on its single-line help row, in the
// order they should appear. Status is the optional muted line
// between the body and the footer; pass the empty string when
// the screen has nothing to report.
//
// The footer always renders on exactly one line. The screen
// picks the bindings it wants visible at a glance, and the
// rest live in the full-help overlay the app shows when the
// user presses the global help key. A footer that never grows
// keeps the body height stable across resizes and across
// state transitions, so the content does not jump around when
// the screen's status changes.
func (f Frame) Render(width, height int, status string, footer []key.Binding, s State) string {
	if width < minFrameWidth {
		width = minFrameWidth
	}
	if height < 1 {
		height = 1
	}

	footerRow := f.renderFooter(width, footer)

	bodyHeight := height - footerHeight
	if status != "" {
		bodyHeight--
	}
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	body := f.renderBody(s)
	body = lipgloss.PlaceVertical(bodyHeight, lipgloss.Top, body)

	parts := []string{body}
	if status != "" {
		parts = append(parts, f.theme.Muted.Render(status))
	}
	parts = append(parts, footerRow)
	return strings.Join(parts, "\n")
}

// FullHelp renders the help overlay the app shows when the
// user presses the global help key. The overlay lists every
// binding the screen exposes, grouped into columns by purpose,
// in the visual style the bubbles help component renders so
// the overlay reads as a polished reference rather than a
// dumped list. The width and height arguments bound the
// overlay to the rectangle it sits inside; the height is the
// runtime's window height minus any chrome the app draws
// around the overlay.
func (f Frame) FullHelp(width, height int, groups [][]key.Binding) string {
	if width < minFrameWidth {
		width = minFrameWidth
	}
	help := f.help.FullHelpView(groups)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		f.theme.Border.Padding(1, 3).Render(help))
}

// minFrameWidth is the floor the frame renders at. Below it
// the terminal is too narrow for any layout that makes sense,
// and the renderer stops shrinking — the terminal clips
// overflow rather than the frame producing a broken line.
const minFrameWidth = 20

// footerHeight is the row count the footer always occupies.
// The footer renders on a single line by design (overflow
// flows into the full-help overlay rather than wrapping), so
// the reservation is one row.
const footerHeight = 1

// renderBody draws the body region for the requested state.
// Each state's prose voice follows the project's rules: full
// sentences with explicit subjects, no semicolons, no AI
// filler, and a next-step that names what the user can do.
func (f Frame) renderBody(s State) string {
	switch s.kind {
	case stateLoading:
		return s.spinner.View()
	case stateEmpty:
		return f.theme.Subtitle.Render(s.headline) +
			"\n\n" +
			f.theme.Muted.Render(s.detail)
	case stateError:
		return f.theme.Error.Render("Could not load: "+s.err.Error()) +
			"\n\n" +
			f.theme.Muted.Render(s.detail)
	case stateReady:
		return s.content
	}
	return ""
}

// renderFooter draws the single-line help row that anchors
// the bottom of the frame. The footer carries the bindings
// the screen passed in, in order, rendered through the
// bubbles help component so the styling matches the full-help
// overlay. The row never wraps — a footer that grew on a
// narrow terminal would also push the body up and create
// layout jitter. A screen that wants to surface more bindings
// than the row can fit leans on the full-help overlay
// instead.
func (f Frame) renderFooter(width int, bindings []key.Binding) string {
	enabled := make([]key.Binding, 0, len(bindings))
	for _, b := range bindings {
		if b.Enabled() {
			enabled = append(enabled, b)
		}
	}
	if len(enabled) == 0 {
		return ""
	}
	model := f.help
	model.SetWidth(width)
	return model.ShortHelpView(enabled)
}
