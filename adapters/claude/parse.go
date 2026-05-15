package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

// readSessionFile is the entry point that turns one Claude session
// file into a Conversation. It opens the file, hands the open
// stream to parseStream, and fills in the project identifier from
// the file path. The split between the two functions is on
// purpose. Tests can call parseStream directly with a fake reader,
// without ever touching a real file. Production code calls
// readSessionFile, which deals with the file part for you.
//
// readSessionFile is the only place that knows how the on-disk
// path maps onto a project identifier, so parseStream stays
// purely about parsing JSONL content and never needs a filesystem
// path to do its work.
func readSessionFile(root fs.FS, sessionFile string, source contracts.StorageVersion) (contracts.Conversation, error) {
	f, err := root.Open(sessionFile)
	if err != nil {
		return contracts.Conversation{}, err
	}
	defer f.Close()
	conv, err := parseStream(f, source)
	if err != nil {
		return contracts.Conversation{}, err
	}
	conv.Project = contracts.ProjectID(projectFolderFromSessionPath(sessionFile))
	return conv, nil
}

// parseStream reads JSONL from r and returns a Conversation. JSONL
// just means "one JSON object per line", which is the format Claude
// Code uses for session files.
//
// The function works in three passes.
//
// First pass: read every line into a small struct called rawRecord
// that grabs only the fields we always need (the type, the UUID,
// the timestamp, and so on). The actual message body stays as raw
// JSON for now, because we do not yet know what shape to expect.
// If a line is not valid JSON at all, we skip it and keep going.
// Crashing on one corrupted line in the middle of an otherwise good
// file would lose every message after it, and that is exactly what
// the resilience rule says we should never do.
//
// Second pass: walk the records in order and turn each one into a
// Message. User and assistant records become real messages. A
// handful of internal Claude records (system notes, attachments,
// queue operations, and a few others) are not part of the
// conversation a person reads, so we drop them on the floor. Records
// of a type chronicle does not recognize at all become a meta
// message that wraps an UnknownBlock holding the original line. The
// renderer can then show the unknown content to the user instead
// of pretending it never existed.
//
// Third pass: sort the messages by timestamp. Claude writes them in
// chronological order today, so the sort almost never reorders
// anything. We do it anyway as a safety net, in case a future Claude
// release writes records out of order for performance reasons.
func parseStream(r io.Reader, source contracts.StorageVersion) (contracts.Conversation, error) {
	// Step 1: read every line of JSONL into a small rawRecord
	// struct that grabs only the fields we always need. The
	// message body stays as raw JSON for now because we do not
	// yet know what shape to expect. A line that is not valid
	// JSON gets skipped so one corrupted line does not lose
	// every later message in the file.
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), scannerBufferMax)

	var records []rawRecord
	for scanner.Scan() {
		var record rawRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		record.line = string(scanner.Bytes())
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return contracts.Conversation{}, err
	}

	// Step 2: walk the records in order and turn each one into
	// a Message. User and assistant records become real
	// messages. A handful of internal Claude records (system
	// notes, attachments, queue operations, and a few others)
	// are not conversation a person reads, so we drop them on
	// the floor. Records of a type chronicle does not recognise
	// at all become a meta message that wraps an UnknownBlock
	// so the renderer can surface them.
	var (
		messages  []contracts.Message
		sessionID contracts.SessionID
		cwd       string
		startedAt time.Time
		endedAt   time.Time
	)
	for _, record := range records {
		if record.SessionID != "" {
			sessionID = contracts.SessionID(record.SessionID)
		}
		if record.Cwd != "" {
			cwd = record.Cwd
		}
		ts, _ := time.Parse(time.RFC3339Nano, record.Timestamp)
		if !ts.IsZero() {
			if startedAt.IsZero() || ts.Before(startedAt) {
				startedAt = ts
			}
			if ts.After(endedAt) {
				endedAt = ts
			}
		}

		switch record.Type {
		case "user":
			messages = append(messages, parseUserRecord(record, ts))
		case "assistant":
			messages = append(messages, parseAssistantRecord(record, ts))
		case "system":
			// System notes are things like local-command output and
			// hook output. They are not part of the conversation a
			// person reads, so we drop them. If a future feature
			// wants to show them, we would add a SystemBlock then.
		case "attachment", "file-history-snapshot", "last-prompt",
			"permission-mode", "queue-operation":
			// Bookkeeping records Claude writes for itself. Not
			// conversation content, so we drop them.
		default:
			// We do not know this record type. The resilience rule
			// says to keep it visible, so we wrap the original line
			// in an UnknownBlock and the renderer surfaces it.
			messages = append(messages, contracts.Message{
				ID:        contracts.MessageID(record.UUID),
				ParentID:  contracts.MessageID(record.ParentUUID),
				Role:      contracts.RoleSystem,
				Timestamp: ts,
				IsMeta:    true,
				Blocks: []contracts.Block{
					contracts.UnknownBlock{Kind: record.Type, Raw: []byte(record.line)},
				},
			})
		}
	}

	// Step 3: sort the messages by timestamp. Claude writes
	// them in chronological order today, so the sort almost
	// never reorders anything. We do it anyway as a safety
	// net, in case a future Claude release writes records out
	// of order for performance reasons.
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].Timestamp.Before(messages[j].Timestamp)
	})

	return contracts.Conversation{
		SessionID:    sessionID,
		Cwd:          cwd,
		StartedAt:    startedAt,
		EndedAt:      endedAt,
		Model:        mostFrequentModel(messages),
		Messages:     messages,
		Capabilities: source.Capabilities,
		Source:       source,
	}, nil
}

// mostFrequentModel returns the model identifier that appears
// on the most assistant messages in the conversation. Claude
// records the model per assistant message, and a single
// session can carry messages from more than one model when the
// user toggles between Sonnet and Opus mid-conversation. The
// most-frequent value is the simplest fair summary, and the
// stats renderer rolls it up into the "by-model" breakdown.
//
// Ties are broken in favor of the model that appeared first in
// the conversation, because sort.SliceStable preserves
// insertion order. The function returns the empty string for a
// conversation with no assistant messages, so the stats
// renderer can place those sessions under "(unknown)".
func mostFrequentModel(messages []contracts.Message) string {
	if len(messages) == 0 {
		return ""
	}
	counts := map[string]int{}
	order := []string{}
	for _, m := range messages {
		if m.Model == "" {
			continue
		}
		if _, seen := counts[m.Model]; !seen {
			order = append(order, m.Model)
		}
		counts[m.Model]++
	}
	best := ""
	bestCount := 0
	for _, name := range order {
		if counts[name] > bestCount {
			best = name
			bestCount = counts[name]
		}
	}
	return best
}

// decodeOrZero unmarshals raw into out and silently swallows any
// error. We use it in places where the resilience contract says
// "give the renderer the best block you can produce, even from
// half-broken JSON". A failed decode leaves out at its zero value,
// which is a usable empty block, and the caller keeps going.
//
// The function exists as a named helper instead of a bare
// `_ = json.Unmarshal(...)` for two reasons. First, it makes the
// intent visible at every call site: the reader sees decodeOrZero
// and knows we are choosing tolerance over strictness here.
// Second, it gives us one place to add diagnostics later (a debug
// counter, a structured log, an opt-in strict mode) without
// hunting through the file for ignore patterns.
func decodeOrZero(raw json.RawMessage, out any) {
	_ = json.Unmarshal(raw, out)
}

// rawRecord holds the small set of fields we read straight from
// every JSONL line. The message body itself stays as raw JSON in
// the Message field, because the right shape to decode it into
// depends on whether the record is a user message or an assistant
// message.
//
// The bits in backticks at the end of each field are called struct
// tags. They tell the JSON decoder which key to read from. Without
// them, Go would look for a JSON key called "Type" instead of
// "type", and the decode would silently set the field to the empty
// value.
//
// The lowercase "line" field at the bottom does not have a tag.
// Lowercase fields in Go are private to the package, and the JSON
// decoder ignores them. We set it ourselves right after the decode,
// so the unknown-record branch in parseStream can keep the original
// line of text around for the renderer.
type rawRecord struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	ParentUUID  string          `json:"parentUuid"`
	SessionID   string          `json:"sessionId"`
	Cwd         string          `json:"cwd"`
	Timestamp   string          `json:"timestamp"`
	IsMeta      bool            `json:"isMeta"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
	line        string
}

// userBody and assistantBody are the two shapes the embedded message
// body can take. Both are written to be forgiving. A field that is
// not in the JSON decodes as the empty value, and a key in the JSON
// that we do not list as a field is ignored. Forgiving decoders are
// what let chronicle keep working when Claude adds a new field in a
// future release.
type userBody struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type assistantBody struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Model   string          `json:"model"`
}

// parseUserRecord turns one user-typed record into a Message. The
// embedded body is decoded into a userBody first, and the content
// field is then handed to decodeUserContent, which knows about the
// two shapes Claude uses for user content.
func parseUserRecord(record rawRecord, ts time.Time) contracts.Message {
	var body userBody
	decodeOrZero(record.Message, &body)

	blocks := decodeUserContent(body.Content)
	return contracts.Message{
		ID:          contracts.MessageID(record.UUID),
		ParentID:    contracts.MessageID(record.ParentUUID),
		Role:        contracts.RoleUser,
		Timestamp:   ts,
		IsMeta:      record.IsMeta,
		IsSidechain: record.IsSidechain,
		Blocks:      blocks,
	}
}

// parseAssistantRecord turns one assistant-typed record into a
// Message. Assistant content is always an array of typed parts in
// Claude's storage, so we go straight to decodeAssistantContent
// without the shape check that decodeUserContent has to do.
//
// Claude records the model identifier on every assistant record,
// so we copy it onto the Message. The session-level model on the
// resulting Conversation is then the most-frequent of these
// per-message values, computed in parseStream after every record
// is in hand.
func parseAssistantRecord(record rawRecord, ts time.Time) contracts.Message {
	var body assistantBody
	decodeOrZero(record.Message, &body)

	blocks := decodeAssistantContent(body.Content)
	return contracts.Message{
		ID:          contracts.MessageID(record.UUID),
		ParentID:    contracts.MessageID(record.ParentUUID),
		Role:        contracts.RoleAssistant,
		Timestamp:   ts,
		IsMeta:      record.IsMeta,
		IsSidechain: record.IsSidechain,
		Model:       body.Model,
		Blocks:      blocks,
	}
}

// decodeUserContent handles the two shapes Claude writes for user
// content. A simple text prompt comes in as a plain string, like
// "How do I read a file?". A richer message comes in as an array
// of typed parts, with text, tool results, or images mixed in.
//
// We tell the two shapes apart by looking at the first byte. A JSON
// string always starts with a double quote, and an array always
// starts with a bracket. So a leading quote means "this is a plain
// string" and we decode it as one. Anything else means "this is an
// array" and we hand it to decodePartArray.
func decodeUserContent(raw json.RawMessage) []contracts.Block {
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && s != "" {
			return []contracts.Block{contracts.TextBlock{Text: s}}
		}
		return nil
	}
	return decodePartArray(raw)
}

// decodeAssistantContent always handles an array of parts, because
// assistant messages in Claude's storage always come in that shape.
// The function is its own helper so the assistant path reads the
// same way as the user path.
func decodeAssistantContent(raw json.RawMessage) []contracts.Block {
	if len(raw) == 0 {
		return nil
	}
	return decodePartArray(raw)
}

// decodePartArray takes a JSON array of content parts and returns
// the matching Block values. Each part has its own shape, so we
// decode the array first and then ask decodePart to handle each
// part one at a time. Parts that decodePart does not recognize come
// back as UnknownBlock values, so we never lose unfamiliar content.
func decodePartArray(raw json.RawMessage) []contracts.Block {
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil
	}
	out := make([]contracts.Block, 0, len(parts))
	for _, p := range parts {
		if block, ok := decodePart(p); ok {
			out = append(out, block)
		}
	}
	return out
}

// decodePart turns one content part into a Block. Each part has a
// "type" field that tells us what kind of part it is, like "text"
// or "tool_use" or "image". We read that field first. Once we know
// the kind, we decode the rest of the part into the matching
// struct. If the kind is one chronicle does not recognize, we wrap
// the original raw JSON in an UnknownBlock so the renderer can
// still show it to the user.
func decodePart(raw json.RawMessage) (contracts.Block, bool) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, false
	}
	switch head.Type {
	case "text":
		var v struct {
			Text string `json:"text"`
		}
		decodeOrZero(raw, &v)
		return contracts.TextBlock{Text: v.Text}, true
	case "thinking":
		var v struct {
			Thinking string `json:"thinking"`
		}
		decodeOrZero(raw, &v)
		return contracts.ThinkingBlock{Text: v.Thinking}, true
	case "tool_use":
		var v struct {
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
		decodeOrZero(raw, &v)
		return contracts.ToolUseBlock{Tool: v.Name, Input: v.Input, CallID: v.ID}, true
	case "tool_result":
		var v struct {
			ToolUseID string          `json:"tool_use_id"`
			Content   json.RawMessage `json:"content"`
			IsError   bool            `json:"is_error"`
		}
		decodeOrZero(raw, &v)
		return contracts.ToolResultBlock{
			CallID:  v.ToolUseID,
			Output:  flattenToolResultContent(v.Content),
			IsError: v.IsError,
		}, true
	case "image":
		var v struct {
			Source struct {
				Type      string `json:"type"`
				MediaType string `json:"media_type"`
				Data      string `json:"data"`
			} `json:"source"`
		}
		decodeOrZero(raw, &v)
		ref := v.Source.Type
		if v.Source.Data != "" {
			ref = fmt.Sprintf("base64:%d bytes", len(v.Source.Data))
		}
		return contracts.ImageBlock{MIME: v.Source.MediaType, PathOrInlineRef: ref}, true
	default:
		return contracts.UnknownBlock{Kind: head.Type, Raw: raw}, true
	}
}

// flattenToolResultContent takes the "content" field of a tool
// result and returns it as a single string. Claude writes this
// content in two different shapes depending on which tool produced
// the result. Some tools write a plain string. Others write an
// array of small parts that look like {type:"text", text:"..."}.
// The rest of chronicle only ever works with one string per
// result, so this helper picks whichever shape was used and turns
// it into that string.
func flattenToolResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		decodeOrZero(raw, &s)
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return string(raw)
	}
	var out string
	for _, p := range parts {
		if p.Type == "text" {
			out += p.Text
		}
	}
	return out
}
