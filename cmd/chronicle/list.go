package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
)

// newListCmd builds the `chronicle list` subcommand. The output is
// JSON Lines, with one session per line. JSON Lines is friendly to
// shell pipelines: tools like `jq -c` or `grep` can filter the
// stream record by record without loading the whole list into
// memory.
func newListCmd() *cobra.Command {
	var providerFlag string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sessions across all detected providers (JSON lines)",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			listings, err := app.ListSessionsAll(providerFlag)
			if err != nil {
				return fail("list: %v", err)
			}
			return writeListings(cmd.OutOrStdout(), listings)
		},
	}
	cmd.Flags().StringVar(&providerFlag, "provider", "", `Limit to one provider, like "claude"`)
	return cmd
}

// writeListings emits one JSON object per line. Each object carries
// the user-friendly fields a shell user is likely to filter on:
// the provider name, the session and project identifiers, the
// title, the timestamps, and the size. The version and fingerprint
// fields help the user pick out sessions written by an unfamiliar
// version of the upstream tool.
func writeListings(w io.Writer, listings []composition.SessionListing) error {
	encoder := json.NewEncoder(w)
	for _, l := range listings {
		out := struct {
			Provider    string `json:"provider"`
			SessionID   string `json:"session_id"`
			ProjectID   string `json:"project_id"`
			Title       string `json:"title"`
			StartedAt   string `json:"started_at,omitempty"`
			LastActive  string `json:"last_active,omitempty"`
			TurnCount   int    `json:"turn_count"`
			SizeBytes   int64  `json:"size_bytes"`
			Version     string `json:"version"`
			Fingerprint string `json:"fingerprint,omitempty"`
		}{
			Provider:    l.Provider,
			SessionID:   string(l.Summary.ID),
			ProjectID:   string(l.Summary.Project),
			Title:       l.Summary.Title,
			StartedAt:   fmtTime(l.Summary.StartedAt),
			LastActive:  fmtTime(l.Summary.LastActive),
			TurnCount:   l.Summary.TurnCount,
			SizeBytes:   l.Summary.SizeBytes,
			Version:     l.Summary.Source.Version,
			Fingerprint: l.Summary.Source.Fingerprint,
		}
		if err := encoder.Encode(out); err != nil {
			return fmt.Errorf("encode: %w", err)
		}
	}
	return nil
}
