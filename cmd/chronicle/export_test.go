package main

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

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
