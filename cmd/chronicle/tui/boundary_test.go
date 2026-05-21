package tui

import (
	"strings"
	"testing"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
)

// TestParseTheme_acceptsKnownNames pins every theme string the
// chronicle config file documents. The chronicle binary reads
// the value from `[ui.tui].theme` and passes it through this
// function; a regression here would silently flip every
// chronicle install over to a different scheme on the next
// release.
func TestParseTheme_acceptsKnownNames(t *testing.T) {
	cases := []struct {
		name string
		want theme.Variant
	}{
		{"", theme.VariantTerminal},
		{"auto", theme.VariantTerminal},
		{"terminal", theme.VariantTerminal},
		{"dark", theme.VariantDark},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ParseTheme(c.name)
			if !ok {
				t.Errorf("ParseTheme(%q) should be recognised", c.name)
			}
			if got != c.want {
				t.Errorf("ParseTheme(%q) = %d, want %d", c.name, got, c.want)
			}
		})
	}
}

// TestParseTheme_rejectsUnknownName confirms the failure path
// the boundary code in main.go branches on when the user wrote
// a typo. The fallback variant has to be VariantTerminal because
// that is the same value the empty-string and "auto" cases
// produce — the runtime gets the chronicle default in every
// failure case.
func TestParseTheme_rejectsUnknownName(t *testing.T) {
	got, ok := ParseTheme("gruvbox")
	if ok {
		t.Error("ParseTheme(\"gruvbox\") should not be recognised")
	}
	if got != theme.VariantTerminal {
		t.Errorf("ParseTheme(\"gruvbox\") fallback = %d, want VariantTerminal (%d)", got, theme.VariantTerminal)
	}
}

// TestIsKnownGlamourStyle_acceptsEveryShippedStyle confirms the
// validator agrees with the knownGlamourStyles slice. The check
// is one-line per style and exists so a future change to the
// slice cannot accidentally drop or rename a value the
// chronicle config file has been documenting.
func TestIsKnownGlamourStyle_acceptsEveryShippedStyle(t *testing.T) {
	for _, name := range knownGlamourStyles {
		t.Run(name, func(t *testing.T) {
			if !IsKnownGlamourStyle(name) {
				t.Errorf("IsKnownGlamourStyle(%q) should accept the documented style", name)
			}
		})
	}
}

// TestIsKnownGlamourStyle_rejectsUnknownName confirms the
// negative path. The boundary code uses the false return to
// trigger the warning that names the valid set.
func TestIsKnownGlamourStyle_rejectsUnknownName(t *testing.T) {
	if IsKnownGlamourStyle("gruvbox") {
		t.Error("IsKnownGlamourStyle(\"gruvbox\") should not be recognised")
	}
}

// TestJoinQuotedOxford_branches pins every branch of the helper
// the JoinKnownThemes and JoinKnownGlamourStyles functions reach
// for. The shapes the boundary code drops into warning sentences
// are: nothing for an empty list, the single quoted value for a
// one-element list, the bare "a or b" for two elements, and the
// Oxford-comma form for three or more elements.
func TestJoinQuotedOxford_branches(t *testing.T) {
	cases := []struct {
		name  string
		items []string
		want  string
	}{
		{"empty list", nil, ""},
		{"one element", []string{"dark"}, `"dark"`},
		{"two elements", []string{"dark", "light"}, `"dark" or "light"`},
		{"three elements", []string{"ascii", "dark", "light"}, `"ascii", "dark", or "light"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := joinQuotedOxford(c.items)
			if got != c.want {
				t.Errorf("joinQuotedOxford(%v) = %q, want %q", c.items, got, c.want)
			}
		})
	}
}

// TestJoinKnownGlamourStyles_includesEveryStyle pins the
// invariant the warning message in main.go depends on: every
// entry in knownGlamourStyles appears in the joined output, so
// the user always sees the complete valid set when their
// config value is rejected.
func TestJoinKnownGlamourStyles_includesEveryStyle(t *testing.T) {
	joined := JoinKnownGlamourStyles()
	for _, name := range knownGlamourStyles {
		if !strings.Contains(joined, `"`+name+`"`) {
			t.Errorf("JoinKnownGlamourStyles output is missing the style %q; got %q", name, joined)
		}
	}
}
