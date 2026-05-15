package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// TestDoctorText_rendersFields proves the text renderer puts each
// piece of provider health in the output. The user reads this text
// to figure out whether chronicle found their data and whether
// anything is wrong, so the format has to be stable.
func TestDoctorText_rendersFields(t *testing.T) {
	healths := []composition.ProviderHealth{{
		Name:         "claude",
		Root:         "/home/u/.claude",
		Version:      contracts.StorageVersion{Version: "claude-1.0", Fingerprint: "abc123"},
		Reachable:    true,
		SessionCount: 42,
	}}
	var buf bytes.Buffer
	if err := writeDoctorText(&buf, healths); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"claude", "/home/u/.claude", "claude-1.0", "abc123", "Sessions:    42"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("doctor text missing %q in:\n%s", want, buf.String())
		}
	}
}

// TestDoctorJSON_isValidJSON confirms the --json output round-trips
// through a JSON decoder. Scripts that consume chronicle doctor
// --json depend on the output being valid JSON, so a regression
// here would break automation.
func TestDoctorJSON_isValidJSON(t *testing.T) {
	healths := []composition.ProviderHealth{{Name: "claude", Reachable: true}}
	var buf bytes.Buffer
	if err := writeDoctorJSON(&buf, healths); err != nil {
		t.Fatal(err)
	}
	var got []composition.ProviderHealth
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("doctor JSON not parseable: %v", err)
	}
	if len(got) != 1 || got[0].Name != "claude" {
		t.Errorf("decoded %v, want one entry named claude", got)
	}
}

// TestDoctorText_emptyHealthsExplains makes sure the no-providers
// case prints an explanation. A blank screen would make the user
// think chronicle had crashed.
func TestDoctorText_emptyHealthsExplains(t *testing.T) {
	var buf bytes.Buffer
	if err := writeDoctorText(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No providers detected") {
		t.Errorf("empty doctor should explain itself, got: %q", buf.String())
	}
}

// TestDoctorText_rendersErrorsAndWarnings confirms that the
// renderer puts errors and warnings on their own labeled lines.
// The user reads "Error:" and "Warning:" prefixes to spot
// trouble at a glance, so the format is part of the user-facing
// contract.
func TestDoctorText_rendersErrorsAndWarnings(t *testing.T) {
	healths := []composition.ProviderHealth{{
		Name:      "claude",
		Reachable: false,
		Errors:    []string{"permission denied: /etc/secret"},
		Warnings:  []string{"unrecognized fingerprint xyz"},
	}}
	var buf bytes.Buffer
	if err := writeDoctorText(&buf, healths); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Error:") || !strings.Contains(out, "permission denied") {
		t.Errorf("error not rendered with Error: label in:\n%s", out)
	}
	if !strings.Contains(out, "Warning:") || !strings.Contains(out, "unrecognized fingerprint") {
		t.Errorf("warning not rendered with Warning: label in:\n%s", out)
	}
}
