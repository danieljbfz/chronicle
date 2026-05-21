// Package sessions renders the master session list — the screen
// the chronicle TUI lands on when a user runs the binary with no
// arguments. The screen reads the cross-provider session list
// from composition.App, sorts it most-recent-first, and lets the
// user filter by substring, navigate with either Vim or arrow
// keys, and open one session's transcript by pressing Enter.
//
// The accessibility bar the project sets binds this screen.
// Every action has a key binding (no mouse is required), the
// help bar at the bottom of the list is always visible, the
// loading, empty, and error states are written as full
// sentences, and the currently focused row uses both an accent
// colour and a bold weight so focus reads through colour or
// weight alone.
package sessions

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/ui"
	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// extraHelpBindings lists the bindings this screen advertises in
// addition to the global ones. Enter opens the highlighted
// session; filter and refresh are session-list specific.
var extraHelpBindings = []key.Binding{
	key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
	key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
}

// Lister is the small subset of composition.App methods the
// session list relies on. Defining it inside the screen lets the
// production code pass a *composition.App and the tests pass a
// fake without dragging the rest of composition along for the
// ride. The interface is satisfied automatically by
// *composition.App through its ListSessionsAll method.
type Lister interface {
	ListSessionsAll(providerName string) ([]composition.SessionListing, error)
}

// OpenRequestMsg is the intent the session list emits when the
// user presses Enter on a populated row. The top-level app model
// consumes the message and routes the user to the transcript
// reader. Phase 2 wires the consumer side. The fields carry the
// minimum the transcript reader needs to load the conversation
// through composition.ReadSession.
type OpenRequestMsg struct {
	SessionID contracts.SessionID
	ProjectID contracts.ProjectID
	Provider  string
}

// status names the screen's current loading state. The three
// values are mutually exclusive — the model is loading, ready,
// or in an error state at any given moment, never two at once.
type status int

const (
	statusLoading status = iota
	statusReady
	statusError
)

// Model is the session list screen's tea.Model. Fields are
// grouped by lifetime: inputs the constructor receives and the
// screen never mutates, derived state the screen owns, and the
// terminal dimensions the runtime sets through WindowSizeMsg.
type Model struct {
	src   Lister
	keys  keys.KeyMap
	theme theme.Theme

	list    list.Model
	spinner ui.Spinner
	status  status
	err     error

	width  int
	height int
}

// New returns a Model in its loading state. Init kicks off the
// asynchronous fetch through src.ListSessionsAll, and the
// "Scanning providers" message stays on screen until the
// fetch's loadedMsg or errMsg arrives.
//
// The list is configured for a single visual register — title
// hidden so the screen's own header is the only top-level
// label, status bar visible so the user always sees the session
// count and any transient status messages, help visible so
// every binding the screen accepts is one keystroke away from
// being discoverable.
func New(src Lister, k keys.KeyMap, t theme.Theme) Model {
	home, _ := os.UserHomeDir()

	l := list.New(nil, newDelegate(t, home), 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(true)
	l.SetStatusBarItemName("session", "sessions")
	l.SetFilteringEnabled(true)
	// The shared ui.HelpBar at the footer is the one place the
	// TUI advertises bindings, so the list's built-in help row
	// would duplicate it. Turning it off keeps every screen on
	// one help line in one canonical style.
	l.SetShowHelp(false)

	return Model{
		src:     src,
		keys:    k,
		theme:   t,
		list:    l,
		spinner: ui.NewSpinner(t, "Scanning providers for sessions…"),
		status:  statusLoading,
	}
}

// Init returns the command that loads sessions for the first
// frame, batched with the spinner's tick command so the loading
// row animates and the elapsed counter updates while the fetch
// is in flight.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetch(), m.spinner.TickCmd())
}

// fetch returns a command that calls into the Lister and yields
// either a loadedMsg or an errMsg, depending on the result. The
// command captures the Lister reference rather than reading it
// from the model at execution time, because tea.Cmd values run
// after Update has already returned the new model.
func (m Model) fetch() tea.Cmd {
	src := m.src
	return func() tea.Msg {
		listings, err := src.ListSessionsAll("")
		if err != nil {
			return errMsg{err: err}
		}
		return loadedMsg{listings: listings}
	}
}

type loadedMsg struct {
	listings []composition.SessionListing
}

type errMsg struct {
	err error
}

// Update advances the screen one frame. The branches handle the
// terminal resize, the asynchronous load result, and the keys the
// screen owns. Every other message — most importantly the list's
// own navigation keys — flows into the embedded list at the end
// of the function.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// The app draws the tab strip and divider above this
		// screen and forwards a height already reduced by that
		// chrome. The screen reserves footerHeight rows for its
		// own divider plus the shared help line and hands the
		// rest to the list.
		listHeight := msg.Height - footerHeight
		if listHeight < 1 {
			listHeight = 1
		}
		m.list.SetSize(msg.Width, listHeight)
	case loadedMsg:
		sortByLastActive(msg.listings)
		items := make([]list.Item, len(msg.listings))
		for i, l := range msg.listings {
			items[i] = sessionItem{listing: l}
		}
		cmd := m.list.SetItems(items)
		m.status = statusReady
		return m, cmd
	case errMsg:
		m.err = msg.err
		m.status = statusError
		return m, nil
	case tea.KeyPressMsg:
		// While the user is typing into the filter input, every
		// keystroke must reach the list so the input captures
		// characters that would otherwise trigger screen-level
		// actions ("r" for refresh, "/" for filter, and so on).
		if m.list.SettingFilter() {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
		switch {
		case key.Matches(msg, m.keys.Enter) && m.status == statusReady:
			item, ok := m.list.SelectedItem().(sessionItem)
			if !ok {
				return m, nil
			}
			return m, openRequest(item.listing)
		case key.Matches(msg, m.keys.Refresh):
			m.status = statusLoading
			m.spinner = ui.NewSpinner(m.theme, "Refreshing the session list…")
			return m, tea.Batch(m.fetch(), m.spinner.TickCmd())
		}
	}

	// The spinner only matters while the screen is loading.
	// Forwarding ticks after the load resolves would leave the
	// glyph animating behind the populated list.
	if m.status == statusLoading {
		var spinCmd tea.Cmd
		m.spinner, spinCmd = m.spinner.Update(msg)
		if spinCmd != nil {
			return m, spinCmd
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// footerHeight is the number of terminal rows the screen reserves
// for its own footer: the divider line above the help bar plus
// the help bar itself.
const footerHeight = 2

// IsFiltering reports whether the user is currently editing the
// filter input. The top-level app model checks this before it
// claims a global keystroke like "q", so a user typing the
// letter "q" into the filter does not accidentally quit the
// program.
func (m Model) IsFiltering() bool {
	return m.list.SettingFilter()
}

// PublishStatusMessage pushes a transient note into the list's
// status bar. The list rotates the message back to the item
// count after a few seconds. The app model calls this to
// surface a short notice — for example, "Transcript reader is
// wiring up" — without pushing the rest of the screen layout
// around. Returning the updated Model alongside the cmd keeps
// the call site consistent with every other Update path.
func (m Model) PublishStatusMessage(s string) (Model, tea.Cmd) {
	cmd := m.list.NewStatusMessage(s)
	return m, cmd
}

// View renders the screen's content as a string. The app draws
// the tab strip above this and wraps the whole frame in a
// tea.View, so the session list owns the body plus its own
// footer (divider plus the shared help bar) beneath the chrome.
func (m Model) View() string {
	return m.renderBody() + "\n" + m.renderFooter()
}

// renderBody returns the screen's content below the app's chrome.
// Each status case produces full-sentence prose so a user who
// lands on the screen mid-state always knows what is happening
// and what to do next.
func (m Model) renderBody() string {
	switch m.status {
	case statusLoading:
		return m.spinner.View()
	case statusError:
		return m.theme.Error.Render("Could not load sessions: "+m.err.Error()) +
			"\n\n" +
			m.theme.Muted.Render("Run `chronicle doctor` for the per-provider diagnostic.")
	case statusReady:
		if len(m.list.Items()) == 0 {
			return m.theme.Subtitle.Render("No sessions found across any detected provider.") +
				"\n\n" +
				m.theme.Muted.Render("Run `chronicle doctor` to check whether the provider roots are reachable, or open a new conversation in your AI tool of choice to seed one.")
		}
		return m.list.View()
	}
	return ""
}

// renderFooter paints the divider and the shared help line that
// sit beneath the body. Every screen uses the same shape, so the
// help row reads identically across the TUI.
func (m Model) renderFooter() string {
	width := m.width
	if width < minFooterWidth {
		width = minFooterWidth
	}
	divider := m.theme.Muted.Render(strings.Repeat("─", width))
	return divider + "\n" + ui.HelpBar(m.theme, m.keys, extraHelpBindings, width)
}

// minFooterWidth is the floor the footer renders at. Below it the
// terminal is too narrow for any layout that makes sense, and the
// terminal will clip overflow rather than the renderer producing
// a broken line.
const minFooterWidth = 20

// openRequest wraps a SessionListing in an OpenRequestMsg and
// returns the result as a tea.Cmd. The list's Update returns the
// command so the runtime can deliver the message to the app
// model on its next pass.
func openRequest(l composition.SessionListing) tea.Cmd {
	msg := OpenRequestMsg{
		SessionID: l.Summary.ID,
		ProjectID: l.Summary.Project,
		Provider:  l.Provider,
	}
	return func() tea.Msg { return msg }
}

// sortByLastActive sorts the slice in place so the most recently
// active session comes first. Two sessions with the same
// LastActive (typically zero values, for sessions that were never
// touched after creation) fall back to StartedAt for the
// tie-break, so the order is at least stable across runs.
func sortByLastActive(s []composition.SessionListing) {
	sort.SliceStable(s, func(i, j int) bool {
		a, b := s[i].Summary.LastActive, s[j].Summary.LastActive
		if a.Equal(b) {
			return s[i].Summary.StartedAt.After(s[j].Summary.StartedAt)
		}
		return a.After(b)
	})
}

// sessionItem wraps one composition.SessionListing so the bubbles
// list can hold it as a list.Item. The wrapper is intentionally
// thin — every render path reaches the listing through the
// listing field rather than through a duplicated cache of
// derived strings — so adding a new column later means changing
// the delegate's Render method and nothing else.
type sessionItem struct {
	listing composition.SessionListing
}

// FilterValue concatenates the sanitised title, the project,
// and the provider so the list's filter accepts any one of
// them as a search term. The sanitisation matters because
// session titles routinely contain embedded newlines from the
// user's first pasted message, and raw newlines inside the
// filter target would let a user "match" a session by typing
// characters that were never actually visible in the title.
func (i sessionItem) FilterValue() string {
	return strings.Join([]string{
		sanitizeSingleLine(i.listing.Summary.Title),
		sanitizeSingleLine(string(i.listing.Summary.Project)),
		i.listing.Provider,
	}, " ")
}

// delegate is the bubbles list's per-row renderer. The row draws
// exactly two terminal lines: a header line with the provider
// badge, the title, and a relative-time-ago label, and a
// subtitle line with the decoded project path in a muted style.
// The list component holds a strict invariant that every Render
// call must paint exactly Height() lines; if the delegate ever
// paints more, the list's viewport math drifts, items overlap,
// and the cursor can move to rows that are no longer visible.
// The sanitiser at the top of Render is the one piece of code
// in this file that enforces that invariant.
type delegate struct {
	theme theme.Theme
	home  string
}

func newDelegate(t theme.Theme, home string) delegate {
	return delegate{theme: t, home: home}
}

const (
	delegateHeight      = 2
	delegateSpacing     = 1
	providerColumnWidth = 11
	minimumTitleWidth   = 12
	rowMarkerSelected   = "▌ "
	rowMarkerUnselected = "  "
	provideTitleGap     = 2
	titleAgeGap         = 2
	projectIndentWidth  = 4
)

func (d delegate) Height() int                             { return delegateHeight }
func (d delegate) Spacing() int                            { return delegateSpacing }
func (d delegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d delegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	s, ok := item.(sessionItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()
	width := m.Width()
	if width < providerColumnWidth+minimumTitleWidth+12 {
		width = providerColumnWidth + minimumTitleWidth + 12
	}

	provider := truncate(sanitizeSingleLine(s.listing.Provider), providerColumnWidth)
	providerPadded := padRight(provider, providerColumnWidth)

	age := composition.HumanAge(s.listing.Summary.LastActive)

	title := sanitizeSingleLine(s.listing.Summary.Title)

	project := displayProjectPath(string(s.listing.Summary.Project), d.home)
	project = sanitizeSingleLine(project)

	// The header line is: marker + provider + gap + title + gap + age.
	// Reserve room for everything except the title, then give the
	// title whatever width remains.
	headerOverhead := lipgloss.Width(rowMarkerUnselected) +
		providerColumnWidth + provideTitleGap +
		titleAgeGap + lipgloss.Width(age)
	titleWidth := width - headerOverhead
	if titleWidth < minimumTitleWidth {
		titleWidth = minimumTitleWidth
	}
	title = truncate(title, titleWidth)

	projectIndent := strings.Repeat(" ", projectIndentWidth)
	projectWidth := width - lipgloss.Width(projectIndent)
	if projectWidth < 1 {
		projectWidth = 1
	}
	project = truncate(project, projectWidth)

	marker := rowMarkerUnselected
	if isSelected {
		marker = rowMarkerSelected
	}

	var headerLine, projectLine string
	if isSelected {
		// The selected row has an accent-coloured bar marker and
		// a bold accent title. Focus reads through colour and
		// weight together, so a user with reduced colour
		// perception still sees the focus from the weight
		// change alone, and a user with reduced contrast still
		// sees it from the marker bar.
		headerLine = d.theme.Accent.Render(marker) +
			d.theme.Accent.Render(providerPadded) +
			strings.Repeat(" ", provideTitleGap) +
			d.theme.Accent.Bold(true).Render(title) +
			strings.Repeat(" ", titleAgeGap) +
			d.theme.Muted.Render(age)
		projectLine = projectIndent + d.theme.Muted.Render(project)
	} else {
		headerLine = marker +
			d.theme.Accent.Render(providerPadded) +
			strings.Repeat(" ", provideTitleGap) +
			d.theme.Title.Render(title) +
			strings.Repeat(" ", titleAgeGap) +
			d.theme.Muted.Render(age)
		projectLine = projectIndent + d.theme.Muted.Render(project)
	}

	fmt.Fprintln(w, headerLine)
	fmt.Fprint(w, projectLine)
}

// sanitizeSingleLine collapses any string that might span more
// than one terminal line into one. Newlines, carriage returns,
// and tabs are replaced with spaces, runs of whitespace are
// collapsed into a single space, and leading and trailing
// whitespace is trimmed. The function is the one place a
// future contributor needs to look to understand why the list's
// per-row height invariant survives titles that arrive with
// embedded line breaks from the user's first pasted message.
func sanitizeSingleLine(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := true // treat the implicit left edge as whitespace, so leading runs are skipped
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		if r == ' ' {
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimRight(b.String(), " ")
}

// displayProjectPath turns a provider's raw ProjectID into the
// path a user recognises. Claude's adapter encodes paths by
// replacing every forward slash with a hyphen and prepending a
// leading hyphen for the root slash, so a project identifier
// like "-Users-djbf-Desktop-work-chronicle" comes back as
// "/Users/djbf/Desktop/work/chronicle". The Copilot adapters
// use opaque hashes that have no decoded form, and those are
// returned as-is. When the decoded path begins with the user's
// home directory, the function replaces the home prefix with
// "~" so the row reads "~/Desktop/work/chronicle" rather than
// the full absolute path.
//
// The decoder is best effort. Claude's encoding loses
// information when the original path contained hyphens — a
// "/Users/djbf/my-project" round-trips through the same
// encoding as "/Users/djbf/my/project". The chronicle CLI
// already accepts the loss, and this function inherits the
// trade-off.
func displayProjectPath(projectID, home string) string {
	if projectID == "" {
		return "(unknown project)"
	}

	decoded := projectID
	if strings.HasPrefix(projectID, "-") {
		decoded = "/" + strings.ReplaceAll(projectID[1:], "-", "/")
	}

	if home != "" && strings.HasPrefix(decoded, home) {
		rest := strings.TrimPrefix(decoded, home)
		if rest == "" || rest[0] == '/' {
			return "~" + rest
		}
	}
	return decoded
}

// truncate clips s to a maximum rune count, appending a single
// ellipsis character when the clip actually dropped runes. The
// rune count keeps multi-byte titles from rendering narrower
// than the budget the caller computed.
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// padRight extends s with spaces so the rendered width matches n.
// The width measurement uses lipgloss.Width rather than len so
// strings that contain wide East Asian characters or already
// carry ANSI escape codes pad to the visual width the caller
// wanted.
func padRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}
