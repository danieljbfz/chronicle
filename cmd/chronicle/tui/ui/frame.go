package ui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
)

// Frame is the single rendering rule every chronicle TUI
// screen composes through. A reader looking at the rendered
// TUI cannot tell which screen drew which row of chrome:
// every loading state, every empty state, every error state,
// and every help-row footer reads the same because there is
// exactly one implementation. A screen's only job is to
// supply its content; the frame draws the rest.
//
// The frame fills a rectangle of width × height with two
// stacked regions. The body region takes every row except
// the single-line footer at the bottom. The footer carries
// a short help row of the bindings most useful in the
// current screen, plus the universal `?` and `q` anchors so
// the user always knows how to open the full help overlay
// and how to leave the program.
//
// The frame does not own the screen's keybindings, its
// loading commands, or the global help overlay. Those live
// in each screen and in the app model, because each one is
// the right home for the concern. The frame is a renderer,
// not a model.
type Frame struct {
	theme theme.Theme
	keys  keys.KeyMap
	help  help.Model
}

// NewFrame returns a Frame configured for the chronicle
// theme. The frame holds no per-render state, so a screen
// can reuse one Frame value across every View call. The
// help component the frame composes is preconfigured for
// the theme's help-row styles, so the help row reads
// identically across every screen the frame renders.
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

// State is the screen's request to the frame: what to draw
// in the body region for this render. The four constructors
// below — Loading, Empty, Error, Ready — produce the four
// possible states. A screen never reaches into the State
// directly; it calls one of the constructors with the data
// it wants the frame to surface.
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

// Loading is the state a screen returns while an
// asynchronous fetch is in flight. The spinner is the
// live-elapsed-time indicator the screen holds for the
// duration of the load; the frame draws its View row
// centered in the body region so the loading message sits
// in the visual centre of the screen rather than pinned to
// a corner with empty space below it.
func Loading(s Spinner) State {
	return State{kind: stateLoading, spinner: s}
}

// Empty is the state a screen returns when the fetch
// succeeded but produced no rows. The headline is a
// one-sentence summary of the absence; the detail is the
// next-step prose the screen wants the user to read. The
// frame centres the block in the body region for the same
// reason it centres the loading row: a short message at the
// top of an empty pane reads as a layout error.
func Empty(headline, detail string) State {
	return State{kind: stateEmpty, headline: headline, detail: detail}
}

// Error is the state a screen returns when the fetch
// failed. The frame paints the error in the theme's error
// style with the underlying error message quoted verbatim,
// then renders the detail prose underneath so the user
// knows the next action. The block is centred for the same
// reason as the loading and empty states.
func Error(err error, detail string) State {
	return State{kind: stateError, err: err, detail: detail}
}

// Ready is the state a screen returns when its content is
// in hand and ready to render. The string is the screen's
// body content, already sized to the body region's expected
// width and height. The screen owns the layout inside that
// content; the frame just renders it.
func Ready(content string) State {
	return State{kind: stateReady, content: content}
}

// Render lays the screen out. The width and height
// arguments are the dimensions of the rectangle the frame
// fills — the same dimensions the screen's model received
// through the runtime's last WindowSizeMsg, minus any
// chrome the app draws above the screen. Bindings are the
// screen-curated keys the help row should advertise; the
// frame appends `?` and `q` so the user always sees how to
// open the full help overlay and how to quit.
//
// The footer always renders on exactly one line, never
// wraps, never truncates within the budget — a screen with
// more bindings than the footer fits leans on the full-help
// overlay (press ?) the app shows for the user who wants
// the complete reference.
func (f Frame) Render(width, height int, bindings []key.Binding, s State) string {
	if width < minFrameWidth {
		width = minFrameWidth
	}
	if height < 1 {
		height = 1
	}

	footerRow := f.renderFooter(width, bindings)
	divider := f.theme.Muted.Render(strings.Repeat("─", width))

	bodyHeight := height - footerHeight - dividerHeight
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	body := f.renderBody(width, bodyHeight, s)
	return body + "\n" + divider + "\n" + footerRow
}

// FullHelp renders the help overlay the app shows when the
// user presses the global help key. The overlay lists every
// binding the screen exposes, grouped into columns by
// purpose, in the visual style the bubbles help component
// renders so the overlay reads as a polished reference
// rather than a dumped list. The width and height arguments
// bound the overlay to the rectangle it sits inside.
func (f Frame) FullHelp(width, height int, groups [][]key.Binding) string {
	if width < minFrameWidth {
		width = minFrameWidth
	}
	content := f.help.FullHelpView(groups)
	box := f.theme.Border.Padding(1, 3).Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// minFrameWidth is the floor the frame renders at. Below
// it the terminal is too narrow for any layout that makes
// sense, and the renderer stops shrinking — the terminal
// clips overflow rather than the frame producing a broken
// line.
const minFrameWidth = 20

// footerHeight is the row count the footer always occupies.
// The footer renders on a single line by design — overflow
// flows into the full-help overlay rather than wrapping —
// so the reservation is one row.
const footerHeight = 1

// dividerHeight is the row count the muted divider above
// the footer occupies. The divider sets a clean visual
// boundary between the body region and the help row, so
// the eye does not read the help bindings as part of the
// content above them.
const dividerHeight = 1

// renderBody draws the body region for the requested
// state. The Ready state passes the screen's content
// through, padded to height with blank rows at the bottom
// when the content is shorter than the region — that keeps
// the footer anchored at the rectangle's bottom edge
// instead of riding up against short content. The Loading,
// Empty, and Error states centre their short messages so
// the body reads as a deliberate state rather than a
// broken page.
func (f Frame) renderBody(width, height int, s State) string {
	switch s.kind {
	case stateReady:
		return lipgloss.Place(width, height, lipgloss.Left, lipgloss.Top, s.content)
	case stateLoading:
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, s.spinner.View())
	case stateEmpty:
		message := f.theme.Subtitle.Render(s.headline) +
			"\n\n" +
			f.theme.Muted.Render(s.detail)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, message)
	case stateError:
		message := f.theme.Error.Render("Could not load: "+s.err.Error()) +
			"\n\n" +
			f.theme.Muted.Render(s.detail)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, message)
	}
	return ""
}

// renderFooter draws the single-line help row at the
// bottom of the frame. The row always ends with the
// universal `?` and `q` anchors so the user can discover
// the full help and quit the program from any screen
// without learning a per-screen key. The screen's
// bindings come first, in the order the screen passed
// them; the anchors come last.
func (f Frame) renderFooter(width int, bindings []key.Binding) string {
	all := make([]key.Binding, 0, len(bindings)+2)
	for _, b := range bindings {
		if b.Enabled() {
			all = append(all, b)
		}
	}
	all = append(all, f.keys.Help, f.keys.Quit)
	model := f.help
	model.SetWidth(width)
	return model.ShortHelpView(all)
}
