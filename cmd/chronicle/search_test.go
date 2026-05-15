package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/steps"
)

// sampleSearchResults builds a small two-result fixture the
// rendering tests share. We keep it as a helper so each test
// can change one field instead of rebuilding the whole slice.
func sampleSearchResults() []composition.SearchResult {
	return []composition.SearchResult{
		{
			Provider:  "claude",
			SessionID: contracts.SessionID("abc"),
			ProjectID: contracts.ProjectID("-Users-x-work"),
			Title:     "How do I read a file in Go?\nfollow-up text",
			Snippets: []steps.SearchSnippet{
				{Role: contracts.RoleUser, Text: "read a file"},
				{Role: contracts.RoleAssistant, Text: "use os.ReadFile"},
			},
		},
		{
			Provider:  "copilot",
			SessionID: contracts.SessionID("def"),
			ProjectID: contracts.ProjectID("ws-1"),
			Title:     "",
			Snippets:  []steps.SearchSnippet{{Role: contracts.RoleUser, Text: "hello"}},
		},
	}
}

// TestWriteSearchText_includesProviderSessionTitleSnippets
// is the rendering happy path. The text format has to put
// the provider, session id, title, and one indented line per
// snippet on the page. The user scans for the title to
// recognise the session, so all four pieces matter.
func TestWriteSearchText_includesProviderSessionTitleSnippets(t *testing.T) {
	var buf bytes.Buffer
	if err := writeSearchText(&buf, sampleSearchResults()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"2 matching session(s)",
		"claude/abc",
		"How do I read a file in Go?",
		"user: read a file",
		"assistant: use os.ReadFile",
		"copilot/def",
		"(no title)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q in:\n%s", want, out)
		}
	}
	// The multi-line title should be reduced to one line so
	// the listing stays aligned. The "follow-up text" line
	// from the fixture should not appear.
	if strings.Contains(out, "follow-up text") {
		t.Error("output should drop trailing title lines, only show first")
	}
}

// TestWriteSearchText_emptyResultsSaysSo confirms the
// no-matches path. Returning early with a one-line message
// is friendlier than printing "0 matching session(s):" and
// then nothing.
func TestWriteSearchText_emptyResultsSaysSo(t *testing.T) {
	var buf bytes.Buffer
	if err := writeSearchText(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No matches found") {
		t.Errorf("output = %q, want a 'no matches' message", buf.String())
	}
}

// TestWriteSearchJSON_emitsOneObjectPerLine pins the JSON
// Lines contract. Scripts pipe `chronicle search --json`
// into jq -c, and that workflow expects exactly one object
// per line with the documented field names.
func TestWriteSearchJSON_emitsOneObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	if err := writeSearchJSON(&buf, sampleSearchResults()); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (one per result)", len(lines))
	}

	var first searchResultJSON
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("first line is not valid JSON: %v", err)
	}
	if first.Provider != "claude" {
		t.Errorf("first.provider = %q, want claude", first.Provider)
	}
	if first.SessionID != "abc" {
		t.Errorf("first.session_id = %q, want abc", first.SessionID)
	}
	if len(first.Snippets) != 2 || first.Snippets[0].Text != "read a file" {
		t.Errorf("first.snippets = %v, want two with the user snippet first", first.Snippets)
	}
}

// TestFirstLine_returnsFirstNonEmptyLine pins the small
// title-trimming helper. We test the empty-input, single-
// line, multi-line, and leading-whitespace cases together
// so the contract is easy to read.
func TestFirstLine_returnsFirstNonEmptyLine(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"only one line", "only one line"},
		{"\n\n  real line  \nignored", "real line"},
		{"first\nsecond", "first"},
	}
	for _, tc := range cases {
		if got := firstLine(tc.in); got != tc.want {
			t.Errorf("firstLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
