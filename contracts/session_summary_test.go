package contracts

import (
	"testing"
	"time"
)

func TestNewSessionSummary_MapsConversationAndRefFields(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	ref := SessionRef{
		ID:        "sess-1",
		Project:   "proj-1",
		SizeBytes: 4096,
		ModTime:   start,
		Locator:   "projects/proj-1/sess-1.jsonl",
	}
	conv := Conversation{
		StartedAt: start,
		EndedAt:   end,
		Model:     "claude-opus-4",
		Messages: []Message{
			{Role: RoleUser, Blocks: []Block{TextBlock{Text: "Hello there"}}},
			{Role: RoleAssistant, Blocks: []Block{TextBlock{Text: "Hi"}}},
		},
	}
	version := StorageVersion{
		Adapter:      "claude",
		Version:      "claude-1.0",
		Fingerprint:  "abc123",
		Capabilities: Capabilities{ModelMetadata: true},
	}

	got := NewSessionSummary(ref, conv, version)

	if got.ID != "sess-1" || got.Project != "proj-1" {
		t.Fatalf("identity not carried: %+v", got)
	}
	if !got.StartedAt.Equal(start) || !got.LastActive.Equal(end) {
		t.Fatalf("timestamps not carried: %+v", got)
	}
	if got.Title != "Hello there" {
		t.Fatalf("title = %q, want the first user prompt", got.Title)
	}
	if got.TurnCount != 2 {
		t.Fatalf("turn count = %d, want 2", got.TurnCount)
	}
	if got.SizeBytes != 4096 {
		t.Fatalf("size = %d, want 4096 from the ref", got.SizeBytes)
	}
	if got.Model != "claude-opus-4" {
		t.Fatalf("model = %q", got.Model)
	}
	if !got.Capabilities.ModelMetadata || got.Source.Fingerprint != "abc123" {
		t.Fatalf("version not carried: %+v", got)
	}
}
