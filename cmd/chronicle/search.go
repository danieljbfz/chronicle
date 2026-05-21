package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
)

// newSearchCmd builds the `chronicle search` subcommand. The
// command takes a substring query, walks every session
// across every detected provider, and prints the matching
// sessions with a short snippet around each match.
//
// The motivating problem: a user with hundreds of sessions
// remembers having a conversation about X but cannot find
// the session. `chronicle list` only surfaces the first
// user prompt, which is rarely what the user remembers
// later. Search closes that gap by looking at the full
// content of every session.
//
// Today the search walks one session at a time, with no
// concurrency. On the contributor's machine that is around
// 180 sessions across two providers, and the walk finishes
// in a few hundred milliseconds. If a future install grows
// to thousands of sessions, we will add either an errgroup
// fan-out or a small index that the search consults instead
// of opening every file. Which of the two we pick depends
// on how a real install at that scale behaves, so we are
// leaving the decision until that scenario actually shows up.
func newSearchCmd() *cobra.Command {
	var (
		providerFlag      string
		caseSensitiveFlag bool
		jsonFlag          bool
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Find sessions containing a substring across every provider",
		Args:  cobra.ExactArgs(1),
		Long: `chronicle search walks every session across every detected
provider and prints the sessions whose user or assistant text
contains the query. The match is case-insensitive by default
(pass --case-sensitive for exact matching) and a substring,
not a regex.

For each matching session, the output shows up to three short
snippets around the match so the user can recognize the right
session at a glance. Tool calls, thinking blocks, and slash-
command echoes are skipped because they are noise for content
search.

Pass --json for machine-readable output, one JSON line per
matching session, suitable for piping into other tools.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			results, err := app.Search(args[0], composition.SearchOptions{
				Provider:      providerFlag,
				CaseSensitive: caseSensitiveFlag,
			})
			if err != nil {
				return fail("search: %v", err)
			}
			if jsonFlag {
				return writeSearchJSON(cmd.OutOrStdout(), results)
			}
			return writeSearchText(cmd.OutOrStdout(), results)
		},
	}
	cmd.Flags().StringVar(&providerFlag, "provider", "", `Limit to one provider by name (see chronicle doctor for the list)`)
	cmd.Flags().BoolVar(&caseSensitiveFlag, "case-sensitive", false, "Match the query case exactly")
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "Emit results as JSON lines instead of text")
	return cmd
}

// writeSearchText renders the search results in the format
// a human reads. Each matching session takes one header
// line, followed by up to three snippets indented under it.
// The format is meant to be skimmed: the user scans the
// titles, picks the right session, and then either reads
// the snippets or runs `chronicle export` on the session id.
func writeSearchText(w io.Writer, results []composition.SearchResult) error {
	if len(results) == 0 {
		fmt.Fprintln(w, "No matches found.")
		return nil
	}
	fmt.Fprintf(w, "%d matching %s:\n\n",
		len(results), composition.Pluralize(len(results), "session", "sessions"))
	for _, result := range results {
		title := firstLine(result.Title)
		if title == "" {
			title = "(no title)"
		}
		fmt.Fprintf(w, "  %s/%s  %s\n", result.Provider, result.SessionID, title)
		for _, snippet := range result.Snippets {
			fmt.Fprintf(w, "    %s: %s\n", snippet.Role, snippet.Text)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// writeSearchJSON emits one JSON object per matching
// session. Scripts that consume `chronicle search --json`
// depend on the shape, so the structure here is part of the
// CLI's public surface.
func writeSearchJSON(w io.Writer, results []composition.SearchResult) error {
	encoder := json.NewEncoder(w)
	for _, result := range results {
		snippets := make([]searchSnippetJSON, 0, len(result.Snippets))
		for _, s := range result.Snippets {
			snippets = append(snippets, searchSnippetJSON{
				Role: string(s.Role),
				Text: s.Text,
			})
		}
		out := searchResultJSON{
			Provider:  result.Provider,
			SessionID: string(result.SessionID),
			ProjectID: string(result.ProjectID),
			Title:     result.Title,
			Snippets:  snippets,
		}
		if err := encoder.Encode(out); err != nil {
			return fmt.Errorf("encode: %w", err)
		}
	}
	return nil
}

// searchResultJSON is the wire shape for `chronicle search
// --json`. We give it explicit JSON tags so a future rename
// of an in-memory field will not silently change the user-
// facing output.
type searchResultJSON struct {
	Provider  string              `json:"provider"`
	SessionID string              `json:"session_id"`
	ProjectID string              `json:"project_id"`
	Title     string              `json:"title"`
	Snippets  []searchSnippetJSON `json:"snippets"`
}

// searchSnippetJSON mirrors steps.SearchSnippet without
// exposing the byte-offset Position field. The CLI consumer
// has no use for it because the snippet text is already
// usable as-is.
type searchSnippetJSON struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// firstLine returns the first non-empty line of a string,
// trimmed of surrounding whitespace. We use it for session
// titles because the title often comes from the first user
// prompt, which can be many lines long. A multi-line title
// in the search output would push every other line out of
// alignment.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
