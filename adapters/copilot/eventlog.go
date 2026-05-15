package copilot

import (
	"encoding/json"
	"fmt"
	"io"
)

// VS Code stores each Copilot chat session as a JSONL event log.
// The first line is always a full snapshot of the session, and
// every line after that is a small patch that mutates the snapshot
// in place. The mutation kinds we know about are:
//
//	kind 0: full snapshot.        {"kind":0, "v": <whole session object>}
//	kind 1: set field.            {"kind":1, "k":["a","b"], "v": <new value>}
//	kind 2: append to array.      {"kind":2, "k":["a","b"], "v": <element>}
//
// To get the current state of a session, we read every line in
// order, apply each one to the in-memory snapshot, and return the
// final result. The parser in parse.go then walks that result.
//
// The kind values come from VS Code's source. New kinds may appear
// in future VS Code releases, and the resilience contract says we
// keep going when we see one we do not recognize. We log a warning
// through the returned slice of unknown kinds, and the caller
// surfaces those to the user through the doctor view.

// snapshotEventKind is the marker on a full-snapshot record.
const snapshotEventKind = 0

// setEventKind is the marker on a "set field at this path" record.
const setEventKind = 1

// appendEventKind is the marker on an "append to array at this
// path" record.
const appendEventKind = 2

// rawEvent is one decoded event from the JSONL stream. The Kind
// tells us which shape to expect. The K field is the JSON path,
// expressed as a slice of keys to walk into the snapshot. The V
// field holds the new value or array element, decoded as a generic
// any so we can plug it into the snapshot map without committing
// to a typed shape.
type rawEvent struct {
	Kind int             `json:"kind"`
	K    []string        `json:"k"`
	V    json.RawMessage `json:"v"`
}

// replayResult holds everything the caller learns from a successful
// replay. The State map is the reconstructed snapshot. The
// UnknownKinds slice holds any event kinds we did not recognize, so
// the doctor view can surface them. The LineCount is how many
// JSONL lines we processed, which the doctor view also reports.
type replayResult struct {
	State        map[string]any
	UnknownKinds []int
	LineCount    int
}

// replay reads the JSONL stream, applies every event in order, and
// returns the reconstructed snapshot state along with diagnostics.
// The function tolerates events that fail to parse and events with
// kinds it does not recognize. Both kinds of trouble get reported
// through the result, and neither stops the replay.
//
// We use a streaming JSON decoder instead of reading the file
// line-by-line. Real Copilot sessions can have single events that
// reach hundreds of megabytes (think very long tool outputs), and a
// line-buffered reader would either overflow its buffer or have to
// allocate the whole line in memory at once. json.Decoder pulls one
// JSON value at a time from the stream, no matter how big.
func replay(r io.Reader) (replayResult, error) {
	decoder := json.NewDecoder(r)

	var (
		state        map[string]any
		unknownKinds []int
		lineCount    int
	)
	unknownSeen := make(map[int]struct{})

	for {
		var event rawEvent
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			// Garbage value in the middle of the stream. We cannot
			// safely keep going because the decoder loses sync, so
			// we stop here and return what we have. The caller
			// gets a usable snapshot through everything we have
			// already replayed.
			return replayResult{
				State:        state,
				UnknownKinds: unknownKinds,
				LineCount:    lineCount,
			}, nil
		}
		lineCount++

		switch event.Kind {
		case snapshotEventKind:
			snapshot, ok := decodeSnapshot(event.V)
			if !ok {
				continue
			}
			state = snapshot

		case setEventKind:
			if state == nil {
				// A set arrived before the initial snapshot. That
				// should never happen in practice, but we tolerate
				// it by ignoring the patch.
				continue
			}
			var value any
			if err := json.Unmarshal(event.V, &value); err != nil {
				continue
			}
			setAtPath(state, event.K, value)

		case appendEventKind:
			if state == nil {
				continue
			}
			var value any
			if err := json.Unmarshal(event.V, &value); err != nil {
				continue
			}
			appendAtPath(state, event.K, value)

		default:
			// Unknown kind. Record it once so the doctor view can
			// report it, and move on.
			if _, seen := unknownSeen[event.Kind]; !seen {
				unknownSeen[event.Kind] = struct{}{}
				unknownKinds = append(unknownKinds, event.Kind)
			}
		}
	}

	return replayResult{
		State:        state,
		UnknownKinds: unknownKinds,
		LineCount:    lineCount,
	}, nil
}

// decodeSnapshot parses the v field of a kind-0 record into a
// generic map. We use a map instead of a typed struct because the
// snapshot has dozens of fields, only a handful of which the
// parser actually reads. A typed struct would force us to declare
// every field VS Code might add in the future.
func decodeSnapshot(raw json.RawMessage) (map[string]any, bool) {
	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, false
	}
	return snapshot, true
}

// setAtPath walks into the state map along the given key path and
// sets the leaf to value. A path of length one updates a top-level
// field. Longer paths walk into nested objects, creating
// intermediate maps as needed.
//
// VS Code's events sometimes include numeric path components for
// array indices. We treat those the same as string keys, because
// JSON object keys can be any string. If a real array index
// appears, the caller will read the value out through the same
// numeric key when it walks the state.
func setAtPath(state map[string]any, keys []string, value any) {
	if len(keys) == 0 {
		return
	}
	current := state
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]
		next, ok := current[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
	current[keys[len(keys)-1]] = value
}

// appendAtPath walks into the state map along the key path and
// appends value to the slice at the leaf. If the leaf does not
// exist yet, we create a new slice with the one element. If the
// leaf is not a slice, we replace it with a new slice and log no
// error: the resilience contract says we keep going when the data
// surprises us.
func appendAtPath(state map[string]any, keys []string, value any) {
	if len(keys) == 0 {
		return
	}
	current := state
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]
		next, ok := current[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
	leafKey := keys[len(keys)-1]
	existing, _ := current[leafKey].([]any)
	current[leafKey] = append(existing, value)
}

// snapshotString reads a string field from the snapshot map at the
// given key path. We use it inside the parser to pull out fields
// like sessionId, customTitle, and so on. The function returns the
// empty string when the field is missing or not a string.
func snapshotString(state map[string]any, keys ...string) string {
	if v, ok := snapshotAt(state, keys).(string); ok {
		return v
	}
	return ""
}

// snapshotInt reads an integer field from the snapshot. JSON
// numbers decode into Go as float64 by default, so we convert. The
// function returns zero for missing or wrong-typed fields.
func snapshotInt(state map[string]any, keys ...string) int64 {
	switch v := snapshotAt(state, keys).(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return 0
}

// snapshotSlice reads a slice field from the snapshot. The result
// is a slice of any, because the elements can be different shapes.
// The function returns nil for missing or wrong-typed fields.
func snapshotSlice(state map[string]any, keys ...string) []any {
	if v, ok := snapshotAt(state, keys).([]any); ok {
		return v
	}
	return nil
}

// snapshotAt walks the key path and returns whatever value sits
// there, as a generic any. The helper is the building block for
// snapshotString, snapshotInt, and snapshotSlice.
func snapshotAt(state map[string]any, keys []string) any {
	var cur any = state
	for _, key := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[key]
	}
	return cur
}

// summarize returns a short human-readable description of the
// replay result. The doctor view uses this when it wants to
// surface unknown event kinds to the user without dumping the
// entire integer slice.
func (r replayResult) summarize() string {
	if len(r.UnknownKinds) == 0 {
		return ""
	}
	return fmt.Sprintf("Saw %d unknown event kind(s) during replay: %v", len(r.UnknownKinds), r.UnknownKinds)
}
