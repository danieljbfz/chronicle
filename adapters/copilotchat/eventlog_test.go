package copilotchat

import (
	"strings"
	"testing"
)

// TestReplay_appliesSetEvent confirms that a kind-1 event mutates
// the snapshot in place. The fixture snapshot has an empty
// responderUsername, and the kind-1 event sets it to a real value.
func TestReplay_appliesSetEvent(t *testing.T) {
	stream := strings.NewReader(`{"kind":0,"v":{"sessionId":"abc","responderUsername":""}}
{"kind":1,"k":["responderUsername"],"v":"GitHub Copilot"}
`)
	result, err := replay(stream)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotString(result.State, "responderUsername"); got != "GitHub Copilot" {
		t.Errorf("responderUsername = %q, want %q", got, "GitHub Copilot")
	}
}

// TestReplay_appliesAppendEvent confirms that a kind-2 event
// appends to the slice at the given key path. The fixture starts
// with an empty requests array and the kind-2 event adds one
// request to it. VS Code writes the value as an array of elements
// to append, so appending one request is a one-element array.
func TestReplay_appliesAppendEvent(t *testing.T) {
	stream := strings.NewReader(`{"kind":0,"v":{"sessionId":"abc","requests":[]}}
{"kind":2,"k":["requests"],"v":[{"requestId":"r1"}]}
`)
	result, err := replay(stream)
	if err != nil {
		t.Fatal(err)
	}
	requests := snapshotSlice(result.State, "requests")
	if len(requests) != 1 {
		t.Fatalf("requests count = %d, want 1", len(requests))
	}
	first, ok := requests[0].(map[string]any)
	if !ok || first["requestId"] != "r1" {
		t.Errorf("appended request shape wrong: %#v", requests[0])
	}
}

// TestReplay_recordsUnknownKinds is the resilience canary for the
// event-log layer. An unknown event kind must not stop the replay,
// must not corrupt the snapshot, and must show up in
// result.UnknownKinds so the doctor view can surface it.
func TestReplay_recordsUnknownKinds(t *testing.T) {
	stream := strings.NewReader(`{"kind":0,"v":{"sessionId":"abc"}}
{"kind":42,"k":["x"],"v":1}
{"kind":1,"k":["after"],"v":"survived"}
`)
	result, err := replay(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.UnknownKinds) != 1 || result.UnknownKinds[0] != 42 {
		t.Errorf("UnknownKinds = %v, want [42]", result.UnknownKinds)
	}
	if got := snapshotString(result.State, "after"); got != "survived" {
		t.Errorf("kind-1 after the unknown should still apply, got %q", got)
	}
}

// TestReplay_stopsAtMidStreamGarbageButKeepsPrior proves the
// replay returns whatever it had built up so far when the stream
// stops being valid JSON, instead of crashing or losing
// everything. The stream's first event is a clean snapshot, the
// second is literal garbage. The result has the snapshot but not
// the third event that came after the garbage.
//
// This is a difference from the Claude adapter, which uses a line
// scanner and can keep reading after a single bad line. Copilot
// uses a streaming JSON decoder because individual events can be
// hundreds of megabytes (think long tool outputs), and a line
// scanner cannot handle those. The streaming decoder cannot
// resync after a parse error in the middle of the stream, so we
// stop and keep what we already have.
func TestReplay_stopsAtMidStreamGarbageButKeepsPrior(t *testing.T) {
	stream := strings.NewReader(`{"kind":0,"v":{"sessionId":"abc","title":"Original"}}
garbage
{"kind":1,"k":["title"],"v":"NeverApplied"}
`)
	result, err := replay(stream)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotString(result.State, "sessionId"); got != "abc" {
		t.Errorf("sessionId = %q, want abc (snapshot before garbage should survive)", got)
	}
	if got := snapshotString(result.State, "title"); got != "Original" {
		t.Errorf("title = %q, want Original (post-garbage event must not apply)", got)
	}
}

// TestSetAtPath_walksNested confirms that setAtPath creates
// intermediate maps as it walks. The starting state is an empty
// map, and the path is two keys deep, so setAtPath has to invent
// the inner map on the way.
func TestSetAtPath_walksNested(t *testing.T) {
	state := map[string]any{}
	setAtPath(state, []any{"outer", "inner"}, "value")
	inner, ok := state["outer"].(map[string]any)
	if !ok {
		t.Fatalf("outer should be a map, got %#v", state["outer"])
	}
	if inner["inner"] != "value" {
		t.Errorf("inner.inner = %v, want value", inner["inner"])
	}
}

// TestReplay_appliesPatchThroughArrayIndex is the regression guard
// for the empty-session bug. VS Code builds a session's content with
// patches whose path descends through an array index, stored as a
// JSON number, e.g. ["requests", 0, "response"]. The path used to be
// typed as a slice of strings, so the number failed to decode; the
// streaming decoder could not resync, and the whole replay aborted,
// leaving a session whose content arrived through such patches
// completely empty. The replay must instead apply them and reach the
// events that follow.
func TestReplay_appliesPatchThroughArrayIndex(t *testing.T) {
	stream := strings.NewReader(`{"kind":0,"v":{"requests":[{"requestId":"r1"}]}}
{"kind":1,"k":["requests",0,"responseId"],"v":"resp-1"}
{"kind":2,"k":["requests",0,"response"],"v":[{"kind":"markdown"}]}
{"kind":1,"k":["title"],"v":"survived"}
`)
	result, err := replay(stream)
	if err != nil {
		t.Fatal(err)
	}
	// The final string-path event applies only if the integer-path
	// events before it did not abort the replay.
	if got := snapshotString(result.State, "title"); got != "survived" {
		t.Errorf("title = %q, want survived (an integer-index patch must not abort the replay)", got)
	}
	requests := snapshotSlice(result.State, "requests")
	if len(requests) != 1 {
		t.Fatalf("requests count = %d, want 1", len(requests))
	}
	first, ok := requests[0].(map[string]any)
	if !ok {
		t.Fatalf("request[0] should be a map, got %#v", requests[0])
	}
	if first["responseId"] != "resp-1" {
		t.Errorf("set through requests[0].responseId failed: %#v", first["responseId"])
	}
	response, ok := first["response"].([]any)
	if !ok || len(response) != 1 {
		t.Errorf("append through requests[0].response failed: %#v", first["response"])
	}
}
