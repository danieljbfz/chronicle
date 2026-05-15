package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// TestWriteListings_emitsJSONLines confirms the JSON-Lines output
// shape. Two listings should produce two lines, each one a valid
// JSON object with the right fields. The CLI relies on this format
// being stable, because shell pipelines that consume chronicle's
// output expect one record per line.
func TestWriteListings_emitsJSONLines(t *testing.T) {
	var buf bytes.Buffer
	err := writeListings(&buf, []composition.SessionListing{
		{Provider: "claude", Summary: contracts.SessionSummary{
			ID: "abc", Project: "-Users-test-proj", Title: "Hello",
			TurnCount: 3, SizeBytes: 1234,
			Source: contracts.StorageVersion{Version: "claude-1.0", Fingerprint: "abcd1234"},
		}},
		{Provider: "claude", Summary: contracts.SessionSummary{ID: "def"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line 0 is not JSON: %v", err)
	}
	if first["session_id"] != "abc" {
		t.Errorf("session_id = %v, want abc", first["session_id"])
	}
	if first["title"] != "Hello" {
		t.Errorf("title = %v, want Hello", first["title"])
	}
}
