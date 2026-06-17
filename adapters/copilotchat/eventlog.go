package copilotchat

import (
	"encoding/json"
	"errors"
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
// tells us which shape to expect. The K field is the JSON path to
// walk into the snapshot, one component per level. A component is a
// string when it indexes into an object and a number when it indexes
// into an array — VS Code emits both, because a patch can target a
// field deep inside requests[i].response[j]. The components decode
// into the empty interface for that reason: typing K as []string
// would fail to decode the moment a path reached through an array
// index, and because the streaming decoder cannot resync after that
// failure, the whole replay would abort and leave the session empty.
// The V field holds the new value or array element, decoded as a
// generic any so we can plug it into the snapshot without committing
// to a typed shape.
type rawEvent struct {
	Kind int             `json:"kind"`
	K    []any           `json:"k"`
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
			if errors.Is(err, io.EOF) {
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
			// A kind-2 value is the list of elements to append to the
			// array the path names, not a single element. VS Code
			// always writes it as an array — appending one item sends
			// a one-element array — so we extend the target with each
			// element in turn. A value that is not an array (or is
			// null) appends nothing.
			var elems []any
			if err := json.Unmarshal(event.V, &elems); err != nil {
				continue
			}
			for _, elem := range elems {
				appendAtPath(state, event.K, elem)
			}

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

// setAtPath sets value at the given path inside the snapshot. The
// path walks through objects on string components and through arrays
// on numeric ones, so a patch can land a value deep inside, say,
// requests[3].response. The top of the snapshot is always an object,
// so setNode mutates state in place and the discarded return is the
// same map.
func setAtPath(state map[string]any, path []any, value any) {
	if len(path) == 0 {
		return
	}
	setNode(state, path, value)
}

// appendAtPath appends value to the array the path points at, walking
// the same mixed object-and-array path as setAtPath. A missing array
// is created; a leaf that is not an array is replaced by a new one,
// the same keep-going stance the rest of the replayer takes.
func appendAtPath(state map[string]any, path []any, value any) {
	if len(path) == 0 {
		return
	}
	appendNode(state, path, value)
}

// setNode returns node with value placed at path, rebuilding arrays
// and objects on the way back up so a grown or replaced array
// propagates into its parent. An empty path means node itself is the
// target and value replaces it.
func setNode(node any, path []any, value any) any {
	if len(path) == 0 {
		return value
	}
	switch key := path[0].(type) {
	case string:
		m, ok := node.(map[string]any)
		if !ok {
			m = map[string]any{}
		}
		m[key] = setNode(m[key], path[1:], value)
		return m
	case float64:
		idx := int(key)
		if idx < 0 {
			return node
		}
		s := growSlice(node, idx)
		s[idx] = setNode(s[idx], path[1:], value)
		return s
	default:
		// A path component that is neither an object key nor an array
		// index. Skip the patch rather than guess, so a surprising
		// shape is a no-op instead of a corruption.
		return node
	}
}

// appendNode is setNode's sibling for the kind-2 append. It walks to
// the array the path names and appends value to it, creating the
// array when the leaf is missing or not an array.
func appendNode(node any, path []any, value any) any {
	if len(path) == 0 {
		existing, _ := node.([]any)
		return append(existing, value)
	}
	switch key := path[0].(type) {
	case string:
		m, ok := node.(map[string]any)
		if !ok {
			m = map[string]any{}
		}
		m[key] = appendNode(m[key], path[1:], value)
		return m
	case float64:
		idx := int(key)
		if idx < 0 {
			return node
		}
		s := growSlice(node, idx)
		s[idx] = appendNode(s[idx], path[1:], value)
		return s
	default:
		return node
	}
}

// growSlice returns node as a slice long enough to index idx, padding
// with nil entries when the slice is short or absent. The parser
// skips a nil entry when it walks the requests, so padding is safe.
func growSlice(node any, idx int) []any {
	s, _ := node.([]any)
	for len(s) <= idx {
		s = append(s, nil)
	}
	return s
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
