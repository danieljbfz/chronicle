package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// stubProvider is a minimal Provider for the export-command tests.
// It returns a single canned Conversation when asked. We define it
// inside the cmd/chronicle package because export is the only
// caller in this package that needs it.
type stubProvider struct {
	convo contracts.Conversation
}

func (stubProvider) Name() string { return "stub" }
func (stubProvider) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{Adapter: "stub", Version: "stub-1"}, nil
}
func (stubProvider) ListProjects(fs.FS) ([]contracts.Project, error) {
	return nil, nil
}
func (stubProvider) ListSessions(fs.FS, contracts.ProjectID) ([]contracts.SessionSummary, error) {
	return nil, nil
}
func (s stubProvider) ReadSession(_ fs.FS, _ contracts.SessionID) (contracts.Conversation, error) {
	return s.convo, nil
}

// TestRunExport_writesMarkdownToStdoutWhenNoOut checks the default
// output path. With no -o flag, the rendered Markdown ends up on
// the writer the caller passed in. The test passes a bytes.Buffer
// as that writer and confirms the user prompt appears in the
// output.
func TestRunExport_writesMarkdownToStdoutWhenNoOut(t *testing.T) {
	convo := contracts.Conversation{
		SessionID: "abc",
		Source:    contracts.StorageVersion{Adapter: "stub"},
		Messages: []contracts.Message{
			{Role: contracts.RoleUser, Blocks: []contracts.Block{contracts.TextBlock{Text: "hello"}}},
			{Role: contracts.RoleAssistant, Blocks: []contracts.Block{contracts.TextBlock{Text: "hi"}}},
		},
	}
	app := composition.NewForTest([]contracts.Provider{stubProvider{convo: convo}}, []fs.FS{fstest.MapFS{}})
	var buf bytes.Buffer
	if err := runExport(app, "abc", exportOpts{}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("output missing user text: %q", buf.String())
	}
}

// TestBulkExportFilename_includesDatePrefixWhenAvailable
// pins the chronological-ls property. The date comes
// first so files sort correctly without ls -t. The
// session id stays full so two sessions started on the
// same day keep distinct names.
func TestBulkExportFilename_includesDatePrefixWhenAvailable(t *testing.T) {
	got := bulkExportFilename(composition.BulkExportEntry{
		SessionID: "abc-123",
		StartedAt: time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
	})
	if got != "2026-05-15_abc-123.md" {
		t.Errorf("filename = %q, want 2026-05-15_abc-123.md", got)
	}
}

// TestBulkExportFilename_omitsDatePrefixWhenZero proves
// the fallback for sessions without a recorded start
// time. The filename still has to be unique-per-session,
// so we drop just the date component and keep the session
// id with the .md suffix.
func TestBulkExportFilename_omitsDatePrefixWhenZero(t *testing.T) {
	got := bulkExportFilename(composition.BulkExportEntry{
		SessionID: "no-date-session",
	})
	if got != "no-date-session.md" {
		t.Errorf("filename = %q, want no-date-session.md", got)
	}
}

// TestBulkExportFailure_translatesAmbiguousProject covers
// the disambiguation hint. The user typed a project id
// that two providers know, so the message has to point at
// --provider as the fix.
func TestBulkExportFailure_translatesAmbiguousProject(t *testing.T) {
	wrapped := fmt.Errorf("export bulk: project %q: %w", "shared", composition.ErrProjectAmbiguous)
	err := bulkExportFailure("shared", wrapped)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "more than one provider") {
		t.Errorf("err = %q, want it to mention more than one provider", err)
	}
	if !strings.Contains(err.Error(), "--provider") {
		t.Errorf("err = %q, want it to suggest --provider", err)
	}
}

// TestBulkExportFailure_translatesNotFound covers the
// project-not-found branch. The user typed an id that
// exists nowhere, so the message points at chronicle list.
func TestBulkExportFailure_translatesNotFound(t *testing.T) {
	wrapped := fmt.Errorf("export bulk: project %q: %w", "ghost", fs.ErrNotExist)
	err := bulkExportFailure("ghost", wrapped)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "chronicle list") {
		t.Errorf("err = %q, want it to suggest chronicle list", err)
	}
}

// TestBulkExportFailure_passesThroughGenericErrors makes
// sure an unrecognised error still surfaces cleanly. The
// switch should not swallow the underlying message.
func TestBulkExportFailure_passesThroughGenericErrors(t *testing.T) {
	err := bulkExportFailure("p", errors.New("disk on fire"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "disk on fire") {
		t.Errorf("err = %q, want it to mention the original cause", err)
	}
}
