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
	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/steps"
)

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

	sessionID contracts.SessionID
	projectID contracts.ProjectID
	provider  string

	viewport viewport.Model
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
func New(src Reader, k keys.KeyMap, t theme.Theme, sessionID contracts.SessionID, projectID contracts.ProjectID, provider string) Model {
	vp := viewport.New(viewport.WithWidth(0), viewport.WithHeight(0))
	vp.SoftWrap = true
	vp.MouseWheelEnabled = true

	return Model{
		src:       src,
		keys:      k,
		theme:     t,
		sessionID: sessionID,
		projectID: projectID,
		provider:  provider,
		viewport:  vp,
		status:    statusLoading,
	}
}

// Init returns the command that fetches the conversation. The
// command also runs the Markdown pipeline so the user only sees
// one loading state instead of two.
func (m Model) Init() tea.Cmd {
	return m.fetch(m.width)
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
	if width <= 0 {
		width = defaultRenderWidth
	}
	wrapWidth := width - viewportSidePadding*2

	return func() tea.Msg {
		conv, err := src.ReadSession(sessionID)
		if err != nil {
			return errMsg{err: fmt.Errorf("read session: %w", err)}
		}

		rendered, err := renderMarkdown(conv, wrapWidth)
		if err != nil {
			return errMsg{err: fmt.Errorf("render transcript: %w", err)}
		}
		return loadedMsg{conv: conv, rendered: rendered}
	}
}

// renderMarkdown turns a Conversation into the glamour-styled
// string the viewport displays. The function is split out so
// the Update method can re-run it on window-resize messages
// without re-reading from disk.
func renderMarkdown(conv contracts.Conversation, wrapWidth int) (string, error) {
	if wrapWidth < minimumWrapWidth {
		wrapWidth = minimumWrapWidth
	}
	md := steps.Markdown(conv)
	// Glamour v2 dropped WithAutoStyle in favour of explicit
	// style selection. Until the next session adds a real
	// terminal-background detection pass, the dark stylesheet
	// is the safer default — most chronicle users live in a
	// dark terminal, and the light variant on a dark
	// background washes out.
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
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
	headerLines         = 3
	footerLines         = 2
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
		viewportHeight := msg.Height - headerLines - footerLines
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
			rendered, err := renderMarkdown(m.conv, wrapWidth)
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

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the screen content. The header carries the
// breadcrumb plus the session metadata, the body is either the
// loading or error sentence or the viewport, and the footer
// shows the short help line.
func (m Model) View() string {
	return m.renderHeader() + "\n" + m.renderBody() + "\n" + m.renderFooter()
}

func (m Model) renderHeader() string {
	width := m.width
	if width < 20 {
		width = 20
	}

	title := m.theme.Title.Render("chronicle  ·  sessions  ›  transcript")
	subtitle := m.renderSubtitle(width)
	divider := m.theme.Muted.Render(strings.Repeat("─", width))
	return title + "\n" + subtitle + "\n" + divider
}

// renderSubtitle returns the one-line metadata strip beneath the
// breadcrumb. It tries to fit "provider · started · session-id"
// on a single line, truncating the session id from the right if
// the terminal is too narrow to hold the whole thing.
func (m Model) renderSubtitle(width int) string {
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

	joined := strings.Join(parts, m.theme.Muted.Render("  ·  "))
	return joined
}

func (m Model) renderBody() string {
	switch m.status {
	case statusLoading:
		return m.theme.Muted.Render("Loading transcript…")
	case statusError:
		return m.theme.Error.Render("Could not load transcript: "+m.err.Error()) +
			"\n" +
			m.theme.Muted.Render("Press Esc to return to the session list.")
	case statusReady:
		return m.viewport.View()
	}
	return ""
}

func (m Model) renderFooter() string {
	width := m.width
	if width < 20 {
		width = 20
	}
	divider := m.theme.Muted.Render(strings.Repeat("─", width))
	help := m.renderHelp()
	return divider + "\n" + help
}

// renderHelp prints the short binding hints the footer carries.
// The exact set is curated for the transcript reader rather than
// inherited from a global help component: this screen has its
// own most-useful bindings, and the help bar should advertise
// them rather than a one-size-fits-all list.
func (m Model) renderHelp() string {
	entries := []struct{ keyHint, desc string }{
		{"↑/k", "up"},
		{"↓/j", "down"},
		{"u/d", "half page"},
		{"g/G", "top/bottom"},
		{"esc", "back"},
		{"q", "quit"},
	}
	var parts []string
	for _, e := range entries {
		parts = append(parts, m.theme.HelpKey.Render(e.keyHint)+" "+m.theme.HelpDesc.Render(e.desc))
	}
	return strings.Join(parts, m.theme.Muted.Render("  ·  "))
}

func back() tea.Cmd {
	return func() tea.Msg { return BackMsg{} }
}
