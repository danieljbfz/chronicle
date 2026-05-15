package copilotagent

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"sort"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

// scannerBufferMax is the largest single line bufio.Scanner
// will read from events.jsonl. The agent inlines tool
// arguments and tool results into single events, and a
// verbose tool result (a long file read, for example) can
// produce a single line of several hundred kilobytes. We
// cap at 16 MiB, which is more than any realistic event we
// have observed.
const scannerBufferMax = 16 * 1024 * 1024

// File names inside one session directory. We name them as
// constants so the file's place in the layout is documented
// in one location and a future change to the agent's naming
// convention is one edit instead of many.
const (
	eventsFile         = "events.jsonl"
	vscodeMetadataFile = "vscode.metadata.json"
)

// Event types the agent runtime writes today. Naming them
// as constants gives every parser branch one canonical
// reference to test against and lets future maintainers
// grep for who handles each event.
const (
	eventSessionStart          = "session.start"
	eventSessionShutdown       = "session.shutdown"
	eventUserMessage           = "user.message"
	eventAssistantTurnStart    = "assistant.turn_start"
	eventAssistantTurnEnd      = "assistant.turn_end"
	eventAssistantMessage      = "assistant.message"
	eventToolExecutionStart    = "tool.execution_start"
	eventToolExecutionComplete = "tool.execution_complete"
)

// rawEvent is the envelope every event in the agent stream
// shares. The Data field stays as raw JSON because each
// event type has a different inner shape. The second-pass
// parser unmarshals Data into the appropriate typed struct
// once it knows the type.
type rawEvent struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp string          `json:"timestamp"`
	ID        string          `json:"id"`
	ParentID  string          `json:"parentId"`

	// line preserves the original raw text so unknown
	// event types can be wrapped into UnknownBlock without
	// losing fidelity.
	line string
}

type sessionStartData struct {
	SessionID string `json:"sessionId"`
	StartTime string `json:"startTime"`
	Model     string `json:"selectedModel"`
	Context   struct {
		Cwd string `json:"cwd"`
	} `json:"context"`
}

type userMessageData struct {
	Content string `json:"content"`
}

type toolRequestData struct {
	ToolCallID string          `json:"toolCallId"`
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments"`
}

type assistantMessageData struct {
	MessageID    string            `json:"messageId"`
	Content      string            `json:"content"`
	ToolRequests []toolRequestData `json:"toolRequests"`
}

type toolStartData struct {
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	Arguments  json.RawMessage `json:"arguments"`
}

type toolCompleteData struct {
	ToolCallID string          `json:"toolCallId"`
	Success    bool            `json:"success"`
	Result     json.RawMessage `json:"result"`
}

// vscodeMetadata is the optional sidecar VS Code writes
// when it launches a session. We use it only for the
// session title. The authoritative cwd is the one in the
// session.start event.
type vscodeMetadata struct {
	WorkspaceFolder struct {
		FolderPath string `json:"folderPath"`
	} `json:"workspaceFolder"`
	CustomTitle string `json:"customTitle"`
}

// readSession reads one session directory and returns the
// reconstructed Conversation. The function is the entry
// point for both ReadSession on the Provider and the
// listing path that builds SessionSummary values.
//
// We deliberately do not parse checkpoints, files, or
// research subdirectories. Those carry per-turn state and
// attachments that future capabilities (file-history-style
// cleanup, attachment export) can reach for. The current
// read path needs only the event stream and the metadata
// sidecar.
func readSession(root fs.FS, sessionDir string, source contracts.StorageVersion) (contracts.Conversation, error) {
	conv, err := parseEventStream(root, sessionDir, source)
	if err != nil {
		return contracts.Conversation{}, err
	}

	if title := readVscodeTitle(root, sessionDir); title != "" {
		conv.Title = title
	}
	return conv, nil
}

// parseEventStream opens the events.jsonl file and
// replays every event in order, folding them into a
// Conversation. Three passes match the rest of
// chronicle's parsers: scan into raw envelopes, walk the
// envelopes turning each into a Message, and sort the
// messages by timestamp.
func parseEventStream(root fs.FS, sessionDir string, source contracts.StorageVersion) (contracts.Conversation, error) {
	eventsPath := path.Join(sessionDir, eventsFile)
	f, err := root.Open(eventsPath)
	if err != nil {
		return contracts.Conversation{}, newError("read events", eventsPath, err)
	}
	defer f.Close()

	// Step 1: stream every line into a small rawEvent
	// envelope. Lines that fail JSON decoding are skipped
	// rather than aborting the read, the same resilience
	// rule the other adapters follow.
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), scannerBufferMax)

	var events []rawEvent
	for scanner.Scan() {
		raw := scanner.Bytes()
		var ev rawEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			continue
		}
		ev.line = string(raw)
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return contracts.Conversation{}, newError("scan events", eventsPath, err)
	}

	// Step 2: walk the events in order and turn each one
	// into a Message. Tool requests and tool results are
	// joined here so the renderer sees a single
	// conversation and not the underlying event soup.
	var (
		messages   []contracts.Message
		sessionID  contracts.SessionID
		cwd        string
		startedAt  time.Time
		endedAt    time.Time
		toolStarts = map[string]toolStartData{}
	)
	for _, ev := range events {
		ts, _ := time.Parse(time.RFC3339Nano, ev.Timestamp)
		if !ts.IsZero() {
			if startedAt.IsZero() || ts.Before(startedAt) {
				startedAt = ts
			}
			if ts.After(endedAt) {
				endedAt = ts
			}
		}

		switch ev.Type {
		case eventSessionStart:
			var d sessionStartData
			if err := json.Unmarshal(ev.Data, &d); err == nil {
				if d.SessionID != "" {
					sessionID = contracts.SessionID(d.SessionID)
				}
				if d.Context.Cwd != "" {
					cwd = d.Context.Cwd
				}
			}

		case eventUserMessage:
			var d userMessageData
			if err := json.Unmarshal(ev.Data, &d); err != nil || d.Content == "" {
				continue
			}
			messages = append(messages, contracts.Message{
				ID:        contracts.MessageID(ev.ID),
				ParentID:  contracts.MessageID(ev.ParentID),
				Role:      contracts.RoleUser,
				Timestamp: ts,
				Blocks:    []contracts.Block{contracts.TextBlock{Text: d.Content}},
			})

		case eventAssistantMessage:
			var d assistantMessageData
			if err := json.Unmarshal(ev.Data, &d); err != nil {
				continue
			}
			blocks := assistantMessageBlocks(d)
			if len(blocks) == 0 {
				continue
			}
			messages = append(messages, contracts.Message{
				ID:        contracts.MessageID(d.MessageID),
				ParentID:  contracts.MessageID(ev.ParentID),
				Role:      contracts.RoleAssistant,
				Timestamp: ts,
				Blocks:    blocks,
			})

		case eventToolExecutionStart:
			var d toolStartData
			if err := json.Unmarshal(ev.Data, &d); err == nil && d.ToolCallID != "" {
				toolStarts[d.ToolCallID] = d
			}

		case eventToolExecutionComplete:
			var d toolCompleteData
			if err := json.Unmarshal(ev.Data, &d); err != nil || d.ToolCallID == "" {
				continue
			}
			start, ok := toolStarts[d.ToolCallID]
			if !ok {
				// A complete with no preceding start. We
				// surface the result so the user can see
				// the response, with no associated request
				// shape.
				start = toolStartData{ToolCallID: d.ToolCallID}
			}
			messages = append(messages, contracts.Message{
				ID:        contracts.MessageID(ev.ID),
				ParentID:  contracts.MessageID(ev.ParentID),
				Role:      contracts.RoleSystem,
				Timestamp: ts,
				IsMeta:    true,
				Blocks: []contracts.Block{
					contracts.ToolResultBlock{
						CallID:  start.ToolCallID,
						Output:  string(d.Result),
						IsError: !d.Success,
					},
				},
			})
			delete(toolStarts, d.ToolCallID)

		case eventAssistantTurnStart, eventAssistantTurnEnd, eventSessionShutdown:
			// Bookkeeping events the agent uses to bracket
			// its turns. They are not conversation content
			// a person reads, so we drop them.

		default:
			// Unknown event type. The resilience rule says
			// to keep it visible, so we wrap the original
			// line in an UnknownBlock and a meta message.
			messages = append(messages, contracts.Message{
				ID:        contracts.MessageID(ev.ID),
				ParentID:  contracts.MessageID(ev.ParentID),
				Role:      contracts.RoleSystem,
				Timestamp: ts,
				IsMeta:    true,
				Blocks: []contracts.Block{
					contracts.UnknownBlock{Kind: ev.Type, Raw: []byte(ev.line)},
				},
			})
		}
	}

	// Step 3: sort the messages by timestamp. The agent
	// writes events in chronological order today, so the
	// sort almost never reorders anything. We do it
	// anyway as a safety net for future agent versions.
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].Timestamp.Before(messages[j].Timestamp)
	})

	return contracts.Conversation{
		SessionID:    sessionID,
		Cwd:          cwd,
		StartedAt:    startedAt,
		EndedAt:      endedAt,
		Messages:     messages,
		Capabilities: source.Capabilities,
		Source:       source,
	}, nil
}

// assistantMessageBlocks turns one assistant.message data
// payload into the ordered list of contracts.Block values
// chronicle's renderer expects. Text first when present,
// then one ToolUseBlock per tool request. Empty messages
// (no content, no requests) produce an empty slice and the
// caller drops the message.
func assistantMessageBlocks(d assistantMessageData) []contracts.Block {
	var blocks []contracts.Block
	if d.Content != "" {
		blocks = append(blocks, contracts.TextBlock{Text: d.Content})
	}
	for _, req := range d.ToolRequests {
		blocks = append(blocks, contracts.ToolUseBlock{
			CallID: req.ToolCallID,
			Tool:   req.Name,
			Input:  req.Arguments,
		})
	}
	return blocks
}

// readVscodeTitle returns the customTitle from the
// vscode.metadata.json sidecar when it exists. Sessions
// launched from other frontends do not have this file, in
// which case the function returns the empty string and the
// caller falls back to the first user prompt as the title.
func readVscodeTitle(root fs.FS, sessionDir string) string {
	metaPath := path.Join(sessionDir, vscodeMetadataFile)
	data, err := fs.ReadFile(root, metaPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ""
		}
		return ""
	}
	var meta vscodeMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}
	return meta.CustomTitle
}
