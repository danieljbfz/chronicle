// Package transcript renders one session's conversation as
// scrollable rendered Markdown. The screen lands here when the
// user presses Enter on a row in the session list, picks up the
// SessionID from the routing message, asks composition.App for
// the full Conversation, runs the existing steps.Markdown
// pipeline to produce Markdown, and hands the result to glamour
// so the output reads like a polished document rather than a
// plain transcript dump.
//
// The screen meets the same accessibility bar as the session
// list: every action is reachable by keyboard, the help line
// at the bottom is always visible, the loading and error
// states are written as full sentences, and the back action
// is bound to both Esc and "b" so a user with either Vim or
// browser conventions can leave the screen without learning a
// new key.
package transcript

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/ui"
	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/steps"
)

// footerBindings is the screen-curated set the frame's help
// row shows at the bottom of the transcript reader. The set
// is deliberately short — five or six items is the comfort
// range for a single-line footer — and the full list of
// bindings lives in the app's help overlay (press ?) for the
// user who wants to discover everything. Scroll keys (j, k,
// u, d, g, G) are handled by the bubbles viewport directly;
// the footer surfaces only the two-direction j/k hint
// because that is what the new reader needs to know to start.
var footerBindings = []key.Binding{
	key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "scroll")),
	key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}

// Reader is the small subset of composition.App methods the
// transcript reader relies on. Defining the interface here lets
// production code pass a *composition.App and tests pass a fake
// without dragging the rest of composition along.
// *composition.App satisfies the interface through its
// ReadSession method.
type Reader interface {
	ReadSession(id contracts.SessionID) (contracts.Conversation, error)
}

// BackMsg is the intent the transcript reader emits when the
// user presses Esc or b. The top-level app model consumes it
// and routes the user back to the session list. The session
// list keeps the focus row it had when the user pressed Enter,
// so coming back lands on the same session the user just read.
type BackMsg struct{}

// status names the screen's current loading state.
type status int

const (
	statusLoading status = iota
	statusReady
	statusError
)

// Model is the transcript reader screen's tea.Model. The fields
// fall into the same three groups every screen uses: inputs the
// constructor receives and the model never mutates, derived
// state the model owns, and dimensions the runtime sets.
type Model struct {
	src   Reader
	keys  keys.KeyMap
	theme theme.Theme

	// glamourStyle names the Markdown stylesheet the renderer
	// passes to glamour when it produces the rendered output.
	// The value reaches this field from the user's chronicle
	// config through tui.Run and the top-level app model. The
	// transcript reader trusts the value — validation lives at
	// the configuration boundary in cmd/chronicle/main.go.
	glamourStyle string

	sessionID contracts.SessionID
	projectID contracts.ProjectID
	provider  string

	viewport viewport.Model
	frame    ui.Frame
	spinner  ui.Spinner
	status   status
	err      error

	// conv is the raw conversation the loader returns. The
	// model keeps it so window-resize messages can re-render
	// the Markdown at the new width without re-fetching from
	// disk.
	conv contracts.Conversation

	width  int
	height int
}

// New returns a Model in its loading state. Init kicks off the
// asynchronous fetch through src.ReadSession and the subsequent
// Markdown render. The "Loading transcript…" message stays on
// screen until the resulting loadedMsg or errMsg arrives.
func New(src Reader, k keys.KeyMap, t theme.Theme, glamourStyle string, sessionID contracts.SessionID, projectID contracts.ProjectID, provider string) Model {
	vp := viewport.New(viewport.WithWidth(0), viewport.WithHeight(0))
	vp.SoftWrap = true
	vp.MouseWheelEnabled = true

	return Model{
		src:          src,
		keys:         k,
		theme:        t,
		glamourStyle: glamourStyle,
		sessionID:    sessionID,
		projectID:    projectID,
		provider:     provider,
		viewport:     vp,
		frame:        ui.NewFrame(t, k),
		spinner:      ui.NewSpinner(t, "Loading transcript…"),
		status:       statusLoading,
	}
}

// Init returns the command that fetches the conversation. The
// command also runs the Markdown pipeline so the user only sees
// one loading state instead of two. The spinner's tick command
// runs alongside so the loading row animates and the elapsed
// counter updates while the fetch is in flight.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetch(m.width), m.spinner.TickCmd())
}

// fetch returns a command that reads the session, renders the
// Markdown, and produces a loadedMsg with the rendered output.
// Errors at any step (read, glamour construction, glamour
// render) collapse into a single errMsg so the screen has one
// error state rather than three.
//
// The function takes width as a parameter rather than reading
// it from the model, because the model receiver is a value and
// the command runs after Update has already returned. A width
// of zero falls back to a sensible default so the first frame
// (before WindowSizeMsg arrives) still produces something
// readable.
func (m Model) fetch(width int) tea.Cmd {
	src := m.src
	sessionID := m.sessionID
	style := m.glamourStyle
	if width <= 0 {
		width = defaultRenderWidth
	}
	wrapWidth := width - viewportSidePadding*2

	return func() tea.Msg {
		conv, err := src.ReadSession(sessionID)
		if err != nil {
			return errMsg{err: fmt.Errorf("read session: %w", err)}
		}

		rendered, err := renderMarkdown(conv, style, wrapWidth)
		if err != nil {
			return errMsg{err: fmt.Errorf("render transcript: %w", err)}
		}
		return loadedMsg{conv: conv, rendered: rendered}
	}
}

// renderMarkdown turns a Conversation into the glamour-styled
// string the viewport displays. The function is split out so
// the Update method can re-run it on window-resize messages
// without re-reading from disk. The style argument names the
// glamour v2 stylesheet — the configuration boundary in
// cmd/chronicle/main.go validates the value before it reaches
// here, so the renderer trusts it.
func renderMarkdown(conv contracts.Conversation, style string, wrapWidth int) (string, error) {
	if wrapWidth < minimumWrapWidth {
		wrapWidth = minimumWrapWidth
	}
	md := steps.Markdown(conv)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(wrapWidth),
	)
	if err != nil {
		return "", err
	}
	out, err := renderer.Render(md)
	if err != nil {
		return "", err
	}
	return out, nil
}

type loadedMsg struct {
	conv     contracts.Conversation
	rendered string
}

type errMsg struct {
	err error
}

const (
	// headerLines is the row count the transcript's own
	// breadcrumb and metadata header occupies.
	headerLines = 2
	// footerHeight is the row count the frame reserves for
	// the help footer. The frame renders the row on a single
	// line by design.
	footerHeight        = 1
	defaultRenderWidth  = 100
	minimumWrapWidth    = 40
	viewportSidePadding = 2
)

// Update advances the screen one frame. The screen handles the
// window resize, the asynchronous load result, the back action,
// and the top/bottom jumps. The viewport receives every other
// message so the line-by-line and page-by-page navigation that
// the bubbles viewport already implements works out of the box.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		viewportHeight := msg.Height - headerLines - footerHeight
		if viewportHeight < 1 {
			viewportHeight = 1
		}
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(viewportHeight)
		// The Markdown's word-wrap width comes from the
		// terminal width minus the viewport's side padding.
		// Re-render only if we already have a Conversation;
		// otherwise the first render still runs through the
		// fetch path when the load completes.
		if m.status == statusReady {
			wrapWidth := msg.Width - viewportSidePadding*2
			rendered, err := renderMarkdown(m.conv, m.glamourStyle, wrapWidth)
			if err == nil {
				m.viewport.SetContent(rendered)
			}
		}
	case loadedMsg:
		m.conv = msg.conv
		m.viewport.SetContent(msg.rendered)
		m.status = statusReady
		return m, nil
	case errMsg:
		m.err = msg.err
		m.status = statusError
		return m, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Back):
			return m, back()
		case key.Matches(msg, m.keys.Top):
			m.viewport.GotoTop()
			return m, nil
		case key.Matches(msg, m.keys.Bottom):
			m.viewport.GotoBottom()
			return m, nil
		}
	}

	// The spinner only matters while the screen is loading.
	// Forwarding every message would route Bubble Tea's
	// tea.TickMsg events into the spinner forever, leaving the
	// glyph animating behind a ready or error state.
	if m.status == statusLoading {
		var spinCmd tea.Cmd
		m.spinner, spinCmd = m.spinner.Update(msg)
		if spinCmd != nil {
			return m, spinCmd
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the screen content. The transcript draws its
// own two-row breadcrumb-and-subtitle header above the frame
// because the overlay covers the app's tab strip, then hands
// the body and footer to the shared frame so the loading,
// error, and ready states read the same as every other
// screen.
func (m Model) View() string {
	frameHeight := m.height - headerLines
	if frameHeight < 1 {
		frameHeight = 1
	}
	return m.renderHeader() + "\n" + m.frame.Render(m.width, frameHeight, "", footerBindings, m.state())
}

// renderHeader paints the transcript overlay's two-row top
// chrome: a breadcrumb that places the user in the
// navigation, and a metadata strip that names the session
// beneath it. The breadcrumb uses the project's hierarchy
// separator (the chevron) to read as parent → child,
// distinguishing it from the bullet the tab strip uses to
// separate peer sections. Together the two rows replace the
// chrome the tab strip would draw if the transcript were not
// an overlay.
func (m Model) renderHeader() string {
	breadcrumb := m.theme.Title.Render("chronicle") +
		m.theme.Muted.Render(theme.HierarchySeparator) +
		m.theme.Subtitle.Render("sessions") +
		m.theme.Muted.Render(theme.HierarchySeparator) +
		m.theme.Accent.Render("transcript")
	subtitle := m.renderSubtitle()
	return breadcrumb + "\n" + subtitle
}

// renderSubtitle returns the one-line metadata strip that
// names the session inside the transcript overlay. The
// fields are joined by the project's peer separator (the
// bullet) so the line reads the same as every other
// peer-list strip the TUI shows.
func (m Model) renderSubtitle() string {
	parts := []string{m.provider}
	if m.status == statusReady {
		if !m.conv.StartedAt.IsZero() {
			parts = append(parts, m.conv.StartedAt.Format("2006-01-02 15:04"))
		}
		if m.conv.Model != "" {
			parts = append(parts, m.conv.Model)
		}
	}
	parts = append(parts, string(m.sessionID))
	return strings.Join(parts, m.theme.Muted.Render(theme.Separator))
}

// state maps the screen's status flag to the frame's State.
// Loading hands the spinner to the frame; error hands
// full-sentence prose; ready hands the viewport's own
// rendered View. The shape of each branch matches the rules
// the frame imposes on every screen.
func (m Model) state() ui.State {
	switch m.status {
	case statusLoading:
		return ui.Loading(m.spinner)
	case statusError:
		return ui.Error(m.err, "Press Esc to return to the session list.")
	case statusReady:
		return ui.Ready(m.viewport.View())
	}
	return ui.Ready("")
}

func back() tea.Cmd {
	return func() tea.Msg { return BackMsg{} }
}
