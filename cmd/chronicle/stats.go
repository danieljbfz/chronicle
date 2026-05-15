package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
)

// newStatsCmd builds the `chronicle stats` subcommand. Stats
// is the read-only at-a-glance summary of every detected
// provider's history. The output is a one-screen view of
// session counts, message counts, disk usage, the active
// date range, and the top projects by session count.
//
// Stats reads only session summaries, never full sessions.
// That makes the command cheap even on a machine with
// thousands of sessions on disk. The time the command takes
// is the time the providers take to enumerate, plus a few
// hundred microseconds of arithmetic.
func newStatsCmd() *cobra.Command {
	var providerFlag string
	var topN int
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show a one-screen summary of every provider's history",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			stats, err := app.Stats(composition.StatsOptions{
				Provider: providerFlag,
				TopN:     topN,
			})
			if err != nil {
				return fail("stats: %v", err)
			}
			if asJSON {
				return writeStatsJSON(cmd.OutOrStdout(), stats)
			}
			return writeStatsText(cmd.OutOrStdout(), stats)
		},
	}
	cmd.Flags().StringVar(&providerFlag, "provider", "", `Limit to one provider, like "claude"`)
	cmd.Flags().IntVar(&topN, "top", 0, "Number of top projects to show (0 uses the default)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit results as JSON instead of text")
	return cmd
}

// writeStatsText renders the summary in three sections:
// totals, per-provider breakdown, and top projects. The
// format is meant to be skimmed top-to-bottom, with the
// most useful single number (session count) at the top of
// the totals block. We use absolute dates rather than
// relative ones in the active-range line because the user
// often wants to know the actual start of their history.
func writeStatsText(w io.Writer, stats composition.Stats) error {
	fmt.Fprintln(w, "Totals")
	fmt.Fprintf(w, "  Sessions: %s\n", humanInt(stats.Total.Sessions))
	fmt.Fprintf(w, "  Messages: %s\n", humanInt(stats.Total.Messages))
	fmt.Fprintf(w, "  Disk:     %s\n", humanBytes(stats.Total.SizeBytes))
	if rangeLine := dateRange(stats.Total); rangeLine != "" {
		fmt.Fprintf(w, "  Active:   %s\n", rangeLine)
	}
	fmt.Fprintln(w)

	if len(stats.Providers) > 0 {
		fmt.Fprintln(w, "By provider")
		for _, p := range stats.Providers {
			fmt.Fprintf(w, "  %s: %s sessions, %s messages, %s across %d project(s)\n",
				p.Name,
				humanInt(p.Aggregate.Sessions),
				humanInt(p.Aggregate.Messages),
				humanBytes(p.Aggregate.SizeBytes),
				p.Projects,
			)
		}
		fmt.Fprintln(w)
	}

	if len(stats.TopProjects) > 0 {
		fmt.Fprintf(w, "Top %d project(s) by session count\n", len(stats.TopProjects))
		for _, proj := range stats.TopProjects {
			label := proj.Path
			if label == "" {
				label = proj.DisplayName
			}
			if label == "" {
				label = string(proj.ProjectID)
			}
			fmt.Fprintf(w, "  %-8s  %s  (%s sessions, %s)\n",
				proj.Provider,
				label,
				humanInt(proj.Aggregate.Sessions),
				humanBytes(proj.Aggregate.SizeBytes),
			)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "Generated at %s\n", stats.GeneratedAt.Format(time.RFC3339))
	return nil
}

// dateRange formats the OldestAt/NewestAt pair as one short
// line. The function returns the empty string when no
// sessions contributed a timestamp, so the caller can omit
// the line instead of printing a confusing zero-value range.
func dateRange(a composition.Aggregate) string {
	if a.OldestAt.IsZero() || a.NewestAt.IsZero() {
		return ""
	}
	span := a.NewestAt.Sub(a.OldestAt)
	days := int(span.Hours() / 24)
	return fmt.Sprintf("%s -> %s  (%d days)",
		a.OldestAt.Format("2006-01-02"),
		a.NewestAt.Format("2006-01-02"),
		days,
	)
}

// statsJSON is the wire shape of the --json output. The
// in-memory Stats struct uses Go-native time.Time values,
// which marshal cleanly, but defining an explicit envelope
// here pins the JSON keys so a future rename of an internal
// field does not silently break a downstream script.
type statsJSON struct {
	GeneratedAt string             `json:"generated_at"`
	Total       aggregateJSON      `json:"total"`
	Providers   []providerStatJSON `json:"providers"`
	TopProjects []projectStatJSON  `json:"top_projects"`
}

type aggregateJSON struct {
	Sessions  int    `json:"sessions"`
	Messages  int    `json:"messages"`
	SizeBytes int64  `json:"size_bytes"`
	OldestAt  string `json:"oldest_at,omitempty"`
	NewestAt  string `json:"newest_at,omitempty"`
}

type providerStatJSON struct {
	Name      string        `json:"name"`
	Projects  int           `json:"projects"`
	Aggregate aggregateJSON `json:"aggregate"`
}

type projectStatJSON struct {
	Provider    string        `json:"provider"`
	ProjectID   string        `json:"project_id"`
	DisplayName string        `json:"display_name,omitempty"`
	Path        string        `json:"path,omitempty"`
	Aggregate   aggregateJSON `json:"aggregate"`
}

// writeStatsJSON emits one indented JSON object. We use
// indented output (not JSON lines) because stats produces
// a single document, and that document is small enough that
// a human reading it without piping through jq still wants
// the indentation.
func writeStatsJSON(w io.Writer, stats composition.Stats) error {
	out := statsJSON{
		GeneratedAt: stats.GeneratedAt.Format(time.RFC3339),
		Total:       toAggregateJSON(stats.Total),
	}
	for _, p := range stats.Providers {
		out.Providers = append(out.Providers, providerStatJSON{
			Name:      p.Name,
			Projects:  p.Projects,
			Aggregate: toAggregateJSON(p.Aggregate),
		})
	}
	for _, proj := range stats.TopProjects {
		out.TopProjects = append(out.TopProjects, projectStatJSON{
			Provider:    proj.Provider,
			ProjectID:   string(proj.ProjectID),
			DisplayName: proj.DisplayName,
			Path:        proj.Path,
			Aggregate:   toAggregateJSON(proj.Aggregate),
		})
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(out)
}

// toAggregateJSON converts a composition.Aggregate into the
// wire shape. Zero-valued timestamps are emitted as empty
// strings, which combined with the omitempty tags drops them
// from the JSON output entirely.
func toAggregateJSON(a composition.Aggregate) aggregateJSON {
	out := aggregateJSON{
		Sessions:  a.Sessions,
		Messages:  a.Messages,
		SizeBytes: a.SizeBytes,
	}
	if !a.OldestAt.IsZero() {
		out.OldestAt = a.OldestAt.Format(time.RFC3339)
	}
	if !a.NewestAt.IsZero() {
		out.NewestAt = a.NewestAt.Format(time.RFC3339)
	}
	return out
}

// humanInt formats an integer with thousands separators so
// the totals line stays readable at a glance. The function
// handles negative numbers correctly even though chronicle
// never produces them, because the cost of one if-statement
// is less than the cost of finding out the hard way later.
func humanInt(n int) string {
	if n < 0 {
		return "-" + humanInt(-n)
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	// Walk the string from the right, inserting a comma
	// every three digits. We build the result in a small
	// byte slice so we do not allocate intermediate strings.
	parts := make([]byte, 0, len(s)+len(s)/3)
	first := len(s) % 3
	if first > 0 {
		parts = append(parts, s[:first]...)
	}
	for i := first; i < len(s); i += 3 {
		if len(parts) > 0 {
			parts = append(parts, ',')
		}
		parts = append(parts, s[i:i+3]...)
	}
	return string(parts)
}
