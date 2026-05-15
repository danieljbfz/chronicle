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

// readSessionFile parses one Claude session JSONL into a normalized
// Conversation. The function is the bridge between Claude's specific
// on-disk format and the shared contracts every other layer of
// chronicle uses. It opens the file, hands the reader to parseStream,
// and returns whatever parseStream produces. The split exists so the
// parsing logic can be tested without touching the filesystem at all:
// the tests give parseStream a strings.Reader full of fixture data,
// and the production code gives it a real file.
func readSessionFile(root fs.FS, sessionFile string, source contracts.StorageVersion) (contracts.Conversation, error) {
	f, err := root.Open(sessionFile)
	if err != nil {
		return contracts.Conversation{}, err
	}
	defer f.Close()
	return parseStream(f, source)
}

// parseStream reads JSONL from r and returns the normalized
// Conversation. The parsing happens in three phases.
//
// First, we read every line into a generic rawRecord struct that
// captures only the envelope fields chronicle cares about. We keep
// the message body as a json.RawMessage so we can defer its parsing
// until we know what shape to expect. Lines that fail to decode as
// JSON at all get skipped, because the resilience contract says we
// never crash on garbage and a corrupted line in the middle of a
// good file should not bring the whole session down.
//
// Second, we walk the records in order and turn each user, assistant,
// or unknown record into a Message. Records of types we know about
// but do not display (system notes, attachments, queue operations,
// permission-mode changes, last-prompt bookmarks, file-history
// snapshots) get dropped silently because they are not part of the
// human-readable conversation. Truly unknown record types become a
// Message wrapping a single UnknownBlock, which is how the resilience
// contract surfaces them to the renderer.
//
// Third, we sort the resulting messages by timestamp. Claude writes
// the records in chronological order today, so the sort is mostly a
// no-op, but a defensive sort makes tests robust against minor
// reorderings and protects us against any future Claude release that
// might write records out of order for performance reasons.
func parseStream(r io.Reader, source contracts.StorageVersion) (contracts.Conversation, error) {
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
			// System notes (local-command stdout, hook output) are
			// not part of the conversation a user reads. We drop
			// them silently for now and would add a SystemBlock if
			// a future feature wanted to surface them.
		case "attachment", "file-history-snapshot", "last-prompt",
			"permission-mode", "queue-operation":
			// These are the metadata records Claude writes for its
			// own bookkeeping. They are not conversation content,
			// so we drop them silently.
		default:
			// Unknown record type. The resilience contract requires
			// us to surface these instead of dropping them, so we
			// produce a meta system message wrapping an UnknownBlock
			// with the original raw line.
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

	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].Timestamp.Before(messages[j].Timestamp)
	})

	return contracts.Conversation{
		SessionID:    sessionID,
		Project:      contracts.ProjectID(cwd),
		StartedAt:    startedAt,
		EndedAt:      endedAt,
		Messages:     messages,
		Capabilities: source.Capabilities,
		Source:       source,
	}, nil
}

// rawRecord captures the envelope fields chronicle reads directly
// from each JSONL line. Anything else stays in the json.RawMessage
// fields, where we can decode it later based on the record type. The
// struct tags map JSON keys to Go field names, and the lowercase
// "line" field at the bottom is unexported so the JSON decoder
// ignores it; we set it ourselves after the decode so the unknown
// branch in parseStream can preserve the original bytes.
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

// userBody and assistantBody describe the two shapes the embedded
// message payload can take. Both are intentionally permissive:
// missing fields decode as zero values, and unknown fields are
// ignored by the JSON decoder. That permissiveness is what lets the
// parser cope with new fields Claude might add in a future release.
type userBody struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type assistantBody struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// parseUserRecord turns one user-typed JSONL record into a
// contracts.Message. We decode the embedded message body into a
// userBody first, then hand the content field to decodeUserContent,
// which knows how to handle both shapes Claude uses for user content
// (a bare string for simple prompts, an array of typed parts for
// anything richer).
func parseUserRecord(record rawRecord, ts time.Time) contracts.Message {
	var body userBody
	_ = json.Unmarshal(record.Message, &body)

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

// parseAssistantRecord turns one assistant-typed JSONL record into a
// contracts.Message. Assistant content is always an array of typed
// parts in Claude's storage, so we go straight to decodeAssistantContent.
func parseAssistantRecord(record rawRecord, ts time.Time) contracts.Message {
	var body assistantBody
	_ = json.Unmarshal(record.Message, &body)

	blocks := decodeAssistantContent(body.Content)
	return contracts.Message{
		ID:          contracts.MessageID(record.UUID),
		ParentID:    contracts.MessageID(record.ParentUUID),
		Role:        contracts.RoleAssistant,
		Timestamp:   ts,
		IsMeta:      record.IsMeta,
		IsSidechain: record.IsSidechain,
		Blocks:      blocks,
	}
}

// decodeUserContent handles the two shapes Claude writes for user
// messages. A simple text prompt comes in as a bare JSON string, like
// "How do I read a file?", and a richer message comes in as an array
// of typed parts that can mix text, tool_result, and image blocks.
// We decide which shape we are looking at by peeking at the first
// byte: a string starts with a double-quote character.
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
// The function exists as its own helper to keep the parseAssistantRecord
// path symmetric with parseUserRecord.
func decodeAssistantContent(raw json.RawMessage) []contracts.Block {
	if len(raw) == 0 {
		return nil
	}
	return decodePartArray(raw)
}

// decodePartArray decodes a JSON array of typed content parts and
// returns the corresponding Block slice. Each part has its own JSON
// shape, so we delegate the per-part decoding to decodePart. Any
// part decodePart cannot recognize comes back as an UnknownBlock,
// which keeps the resilience contract honored.
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

// decodePart decodes one content part into a Block. We start by
// reading just the "type" field to find out which shape to expect,
// then decode the rest into the right concrete struct. A part with
// a type chronicle does not recognize comes back as an UnknownBlock
// carrying the original raw JSON, so the renderer can still surface
// it to the user for inspection.
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
		_ = json.Unmarshal(raw, &v)
		return contracts.TextBlock{Text: v.Text}, true
	case "thinking":
		var v struct {
			Thinking string `json:"thinking"`
		}
		_ = json.Unmarshal(raw, &v)
		return contracts.ThinkingBlock{Text: v.Thinking}, true
	case "tool_use":
		var v struct {
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
		_ = json.Unmarshal(raw, &v)
		return contracts.ToolUseBlock{Tool: v.Name, Input: v.Input, CallID: v.ID}, true
	case "tool_result":
		var v struct {
			ToolUseID string          `json:"tool_use_id"`
			Content   json.RawMessage `json:"content"`
			IsError   bool            `json:"is_error"`
		}
		_ = json.Unmarshal(raw, &v)
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
		_ = json.Unmarshal(raw, &v)
		ref := v.Source.Type
		if v.Source.Data != "" {
			ref = fmt.Sprintf("base64:%d bytes", len(v.Source.Data))
		}
		return contracts.ImageBlock{MIME: v.Source.MediaType, PathOrInlineRef: ref}, true
	default:
		return contracts.UnknownBlock{Kind: head.Type, Raw: raw}, true
	}
}

// flattenToolResultContent accepts either a plain string or an array
// of {type:"text", text:"..."} parts and returns the concatenated
// text. Claude stores tool results in either shape depending on which
// tool produced the result, and the rest of chronicle should not have
// to care which.
func flattenToolResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		_ = json.Unmarshal(raw, &s)
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
