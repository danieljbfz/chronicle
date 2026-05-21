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

// extraHelpBindings lists the bindings this screen advertises
// beyond the global short help. Enter opens the highlighted
// session and slash opens the bubbles list's filter input.
// Refresh is global (the app handles it), so it does not
// appear here.
var extraHelpBindings = []key.Binding{
	key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
	key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
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
	frame ui.Frame

	list    list.Model
	spinner ui.Spinner
	status  status
	err     error

	width  int
	height int
}

// New returns a Model in its loading state. Init kicks off the
// asynchronous fetch through src.ListSessionsAll. The shared
// ui.Frame draws the loading row, the status indicator, and the
// help footer, so the bubbles list's own equivalents are turned
// off — the frame is the one place those rows are rendered for
// every screen.
func New(src Lister, k keys.KeyMap, t theme.Theme) Model {
	home, _ := os.UserHomeDir()

	l := list.New(nil, newDelegate(t, home), 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)

	return Model{
		src:     src,
		keys:    k,
		theme:   t,
		frame:   ui.NewFrame(t, k),
		list:    l,
		spinner: ui.NewSpinner(t, "Scanning providers for sessions…"),
		status:  statusLoading,
	}
}

// Init returns the command that loads sessions for the first
// frame, batched with the spinner's tick command so the
// loading row animates and the elapsed counter updates while
// the fetch is in flight.
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
		// The frame draws the help footer (one or two rows
		// depending on wrap) and an optional status row above
		// it. The list fills whatever the frame's body region
		// leaves it. WindowSizeMsg arrives with the screen's
		// already-reduced height (the app subtracted its tab
		// strip), so the list's size is the body region width
		// times that height minus the rows the frame reserves.
		listWidth, listHeight := m.bodyDimensions()
		m.list.SetSize(listWidth, listHeight)
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
		}
	}

	// The spinner only matters while the screen is loading.
	// Forwarding ticks after the load resolves would leave the
	// glyph animating behind a populated list.
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

// Refresh returns the model to its loading state and kicks off
// a fresh fetch. The app calls this on the global refresh key
// (r) so every section refreshes through one uniform path.
func (m Model) Refresh() (Model, tea.Cmd) {
	m.status = statusLoading
	m.spinner = ui.NewSpinner(m.theme, "Refreshing the session list…")
	return m, tea.Batch(m.fetch(), m.spinner.TickCmd())
}

// IsFiltering reports whether the user is currently editing the
// filter input. The top-level app model checks this before it
// claims a global keystroke like "q", so a user typing the
// letter "q" into the filter does not accidentally quit the
// program.
func (m Model) IsFiltering() bool {
	return m.list.SettingFilter()
}

// View renders the screen through the shared frame so the
// session list draws the same loading, empty, error, footer,
// and status chrome every other screen draws. The screen owns
// only the body content; the frame owns the rest.
func (m Model) View() string {
	return m.frame.Render(m.width, m.height, m.statusLine(), extraHelpBindings, m.state())
}

// state maps the screen's status flag to the frame's State.
// Loading hands the spinner to the frame; empty and error
// hand full-sentence prose; ready hands the list's own
// rendered View. The shape of each branch matches the rules
// the frame imposes on every screen.
func (m Model) state() ui.State {
	switch m.status {
	case statusLoading:
		return ui.Loading(m.spinner)
	case statusError:
		return ui.Error(m.err, "Run `chronicle doctor` for the per-provider diagnostic.")
	case statusReady:
		if len(m.list.Items()) == 0 {
			return ui.Empty(
				"No sessions found across any detected provider.",
				"Run `chronicle doctor` to check whether the provider roots are reachable, or open a new conversation in your AI tool of choice to seed one.",
			)
		}
		return ui.Ready(m.list.View())
	}
	return ui.Ready("")
}

// statusLine is the muted row the frame paints between the
// body and the footer. The session list uses it to report the
// row count so the reader sees the scale at a glance. The
// empty string suppresses the row when there is nothing
// useful to report (loading, error, empty list).
func (m Model) statusLine() string {
	if m.status != statusReady {
		return ""
	}
	count := len(m.list.Items())
	if count == 0 {
		return ""
	}
	return fmt.Sprintf("%d %s", count, composition.Pluralize(count, "session", "sessions"))
}

// bodyDimensions reports the (width, height) the list should
// size itself to. The list fills the frame's body region, so
// its height is the screen's full height minus the rows the
// frame reserves for the footer and the optional status row.
func (m Model) bodyDimensions() (int, int) {
	width := m.width
	height := m.height - footerHeight
	if m.statusLine() != "" {
		height--
	}
	if height < 1 {
		height = 1
	}
	return width, height
}

// footerHeight is the row count the frame reserves for its
// help footer. The frame renders the row on a single line by
// design — overflow flows into the full-help overlay rather
// than wrapping — so the screen reserves one row.
const footerHeight = 1

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
