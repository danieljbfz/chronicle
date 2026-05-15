package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRootCmd_versionFlag confirms the --version flag prints our
// version string. If anyone wires the version in differently
// later, this test catches the regression.
func TestRootCmd_versionFlag(t *testing.T) {
	cmd := newRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), version) {
		t.Errorf("--version output = %q, want it to contain %q", buf.String(), version)
	}
}

// TestRootCmd_helpListsSubcommands proves --help mentions every
// subcommand chronicle ships today. If a subcommand goes missing
// from the registration in newRootCmd, this test fails.
func TestRootCmd_helpListsSubcommands(t *testing.T) {
	cmd := newRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute --help: %v", err)
	}
	for _, want := range []string{"list", "export", "copy", "doctor", "search", "clean", "trash", "memory"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("--help missing subcommand %q in:\n%s", want, buf.String())
		}
	}
}
