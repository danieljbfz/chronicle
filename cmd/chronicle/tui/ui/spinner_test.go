package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
)

// TestSpinner_ViewIncludesMessageAndElapsedTime pins the row
// shape the loading state depends on: the spinner's animated
// glyph, the screen's message, and the parenthesised elapsed-
// time reading. The motionless-loading-message problem this
// component is built to fix is the absence of the elapsed
// reading; the test asserts it lands in the output.
func TestSpinner_ViewIncludesMessageAndElapsedTime(t *testing.T) {
	s := NewSpinner(theme.New(theme.VariantTerminal), "Loading…")
	got := s.View()

	if !strings.Contains(got, "Loading…") {
		t.Errorf("spinner view should include the message; got %q", got)
	}
	if !strings.Contains(got, "(") || !strings.Contains(got, " s)") {
		t.Errorf("spinner view should include a parenthesised seconds reading; got %q", got)
	}
}

// TestFormatElapsed_subMinuteUsesSeconds and the sibling test
// below pin the two-tier elapsed format. Under a minute the
// row reads "(2.5 s)"; at or above a minute it switches to
// "Xm YYs" so a multi-minute load stays under ten characters.
func TestFormatElapsed_subMinuteUsesSeconds(t *testing.T) {
	got := formatElapsed(2500 * time.Millisecond)
	if got != "(2.5 s)" {
		t.Errorf("formatElapsed(2.5s) = %q, want %q", got, "(2.5 s)")
	}
}

func TestFormatElapsed_atOrAboveMinuteSwitchesUnits(t *testing.T) {
	got := formatElapsed(time.Minute + 4*time.Second)
	if got != "(1m 04s)" {
		t.Errorf("formatElapsed(1m4s) = %q, want %q", got, "(1m 04s)")
	}
}
