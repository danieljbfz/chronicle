package copilotchat

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strconv"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

// readSessionFile is the entry point that turns one Copilot session
// file into a Conversation. It opens the file, hands the stream to
// the event-log replayer, and then asks parseSnapshot to turn the
// reconstructed snapshot into the shared domain types. The split
// keeps each piece focused on one job.
func readSessionFile(root fs.FS, sessionFile string, project contracts.ProjectID, source contracts.StorageVersion) (contracts.Conversation, error) {
	f, err := root.Open(sessionFile)
	if err != nil {
		return contracts.Conversation{}, err
	}
	defer f.Close()

	result, err := replay(f)
	if err != nil {
		return contracts.Conversation{}, err
	}
	if result.State == nil {
		// The file contained no snapshot record. We still return a
		// usable empty Conversation so the caller can list it, even
		// though there is nothing inside to render.
		return contracts.Conversation{Source: source, Project: project}, nil
	}
	return parseSnapshot(result.State, project, source), nil
}

// parseSnapshot turns the replayed snapshot map into a
// Conversation. The snapshot has a handful of top-level fields we
// care about (the session identifier, the timestamps, the optional
// custom title) and one big requests array that holds the actual
// conversation. Every entry in requests becomes one user Message
// followed by one assistant Message, mirroring how Copilot stores
// each prompt-and-reply pair as a single bundled record.
func parseSnapshot(state map[string]any, project contracts.ProjectID, source contracts.StorageVersion) contracts.Conversation {
	id := contracts.SessionID(snapshotString(state, "sessionId"))
	startedAt := epochMillisToTime(snapshotInt(state, "creationDate"))
	endedAt := epochMillisToTime(snapshotInt(state, "lastMessageDate"))
	title := snapshotString(state, "customTitle")

	var messages []contracts.Message
	for index, entry := range snapshotSlice(state, "requests") {
		request, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		userMessage, assistantMessage := parseRequestPair(request, index)
		if userMessage != nil {
			messages = append(messages, *userMessage)
		}
		if assistantMessage != nil {
			messages = append(messages, *assistantMessage)
		}
	}

	return contracts.Conversation{
		SessionID:    id,
		Project:      project,
		StartedAt:    startedAt,
		EndedAt:      endedAt,
		Title:        title,
		Messages:     messages,
		Capabilities: source.Capabilities,
		Source:       source,
	}
}

// parseRequestPair turns one entry from the requests array into the
// user message that the user typed and the assistant message that
// Copilot produced in reply. We return both as pointers so the
// caller can treat a missing user or missing assistant as a normal
// case and skip it cleanly.
//
// The index argument is the position of this request in the
// snapshot's requests array. We synthesize stable Message IDs from
// it because Copilot does not assign per-message identifiers the
// way Claude does.
func parseRequestPair(request map[string]any, index int) (*contracts.Message, *contracts.Message) {
	requestID := snapshotString(request, "requestId")
	if requestID == "" {
		requestID = strconv.Itoa(index)
	}
	timestamp := epochMillisToTime(snapshotInt(request, "timestamp"))

	userBlocks := decodeUserMessage(request)
	assistantBlocks := decodeAssistantResponse(request)

	var userMessage, assistantMessage *contracts.Message
	if len(userBlocks) > 0 {
		userMessage = &contracts.Message{
			ID:        contracts.MessageID(requestID + ":user"),
			Role:      contracts.RoleUser,
			Timestamp: timestamp,
			Blocks:    userBlocks,
		}
	}
	if len(assistantBlocks) > 0 {
		assistantMessage = &contracts.Message{
			ID:        contracts.MessageID(requestID + ":assistant"),
			ParentID:  contracts.MessageID(requestID + ":user"),
			Role:      contracts.RoleAssistant,
			Timestamp: timestamp,
			Blocks:    assistantBlocks,
		}
	}
	return userMessage, assistantMessage
}

// decodeUserMessage pulls the user-typed content out of a request
// entry. Copilot stores the user message under request.message,
// which is itself an object with a parts array. Each part has a
// kind field that tells us what flavour of input it is: plain
// text, an @-agent invocation, a /slash command, or a #variable.
// We render each kind as a TextBlock with its prefix included, so
// the exported transcript reads the way the user originally typed
// it.
func decodeUserMessage(request map[string]any) []contracts.Block {
	message, ok := request["message"].(map[string]any)
	if !ok {
		return nil
	}
	parts, ok := message["parts"].([]any)
	if !ok {
		return nil
	}

	var text string
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := part["kind"].(string)
		switch kind {
		case "text":
			text += stringField(part, "text")
		case "agent":
			// @workspace, @terminal, and so on. The agent name is
			// in part.agent.name in some versions and in
			// part.agent.id in others. Try both, settle for either.
			agentName := stringField(nestedMap(part, "agent"), "name")
			if agentName == "" {
				agentName = stringField(nestedMap(part, "agent"), "id")
			}
			if agentName != "" {
				text += "@" + agentName + " "
			}
		case "slash":
			text += "/" + stringField(part, "name") + " "
		case "var":
			text += "#" + stringField(part, "name") + " "
		default:
			// Unknown part kind. Surface it so the user can see
			// something happened, then keep going.
			raw, _ := json.Marshal(part)
			text += fmt.Sprintf("[unknown part %q: %s] ", kind, string(raw))
		}
	}

	if text == "" {
		return nil
	}
	return []contracts.Block{contracts.TextBlock{Text: text}}
}

// decodeAssistantResponse pulls the assistant reply out of a
// request entry. Copilot stores the response as an array of typed
// parts under request.response. Each part has a kind field, and
// we map them to chronicle's Block types.
//
// The mapping is roughly:
//
//	markdown  ->  TextBlock
//	thinking  ->  ThinkingBlock
//	tool      ->  ToolUseBlock plus an optional ToolResultBlock
//	UI-only   ->  dropped (progress messages, confirmations, etc.)
//	other     ->  UnknownBlock holding the raw JSON
//
// One quirk: Copilot also writes raw markdown values that have no
// kind field at all but do have value/baseUri/supportHtml fields.
// These are VS Code's MarkdownString values inlined into the
// response array. We treat those as markdown.
func decodeAssistantResponse(request map[string]any) []contracts.Block {
	parts, ok := request["response"].([]any)
	if !ok {
		return nil
	}

	var blocks []contracts.Block
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind, hasKind := part["kind"].(string)

		// MarkdownString values written without a kind field are
		// the most common assistant content on long sessions. We
		// recognize them by their presence of "value" without a
		// "kind", which is the shape VS Code's MarkdownString
		// serializes to.
		if !hasKind || kind == "" {
			text := decodeMarkdownPart(part)
			if text != "" {
				blocks = append(blocks, contracts.TextBlock{Text: text})
			}
			continue
		}

		switch kind {
		case "markdown":
			text := decodeMarkdownPart(part)
			if text != "" {
				blocks = append(blocks, contracts.TextBlock{Text: text})
			}
		case "thinking":
			text := stringField(part, "value")
			if text != "" {
				blocks = append(blocks, contracts.ThinkingBlock{Text: text})
			}
		case "toolInvocation", "toolInvocationSerialized":
			toolUse, toolResult := decodeToolInvocation(part)
			if toolUse != nil {
				blocks = append(blocks, *toolUse)
			}
			if toolResult != nil {
				blocks = append(blocks, *toolResult)
			}
		case "progressMessage", "progressTaskSerialized",
			"confirmation", "commandButton", "undoStop",
			"inlineReference", "codeblockUri", "textEdit",
			"notebookEdit", "textEditGroup", "mcpServersStarting",
			"elicitationSerialized":
			// UI-only response parts. They carry status, button
			// definitions, edit tracking, and so on. None of it
			// belongs in a transcript a person reads, so we drop
			// these silently.
		default:
			rawJSON, _ := json.Marshal(part)
			blocks = append(blocks, contracts.UnknownBlock{
				Kind: kind,
				Raw:  rawJSON,
			})
		}
	}
	return blocks
}

// decodeMarkdownPart pulls the text out of a markdown response
// part. Copilot stores the text under part.value.value (an object
// wrapped around the raw string) in current versions, and under
// part.content in older ones. We try both shapes and use whichever
// one we find first.
func decodeMarkdownPart(part map[string]any) string {
	if value, ok := part["value"].(map[string]any); ok {
		if text, ok := value["value"].(string); ok {
			return text
		}
	}
	if text, ok := part["content"].(string); ok {
		return text
	}
	if text, ok := part["value"].(string); ok {
		return text
	}
	return ""
}

// decodeToolInvocation turns a Copilot tool-invocation response
// part into a ToolUseBlock plus an optional ToolResultBlock. The
// tool call itself is one block, and if Copilot already recorded
// the outcome (which it does for most synchronous tools), the
// outcome becomes a second block linked by call ID.
func decodeToolInvocation(part map[string]any) (*contracts.ToolUseBlock, *contracts.ToolResultBlock) {
	name := stringField(part, "toolName")
	if name == "" {
		name = stringField(part, "name")
	}
	callID := stringField(part, "toolCallId")
	if callID == "" {
		callID = stringField(part, "id")
	}

	rawInput, _ := json.Marshal(part["parameters"])
	use := &contracts.ToolUseBlock{
		Tool:   name,
		Input:  rawInput,
		CallID: callID,
	}

	resultText := decodeToolResult(part)
	if resultText == "" {
		return use, nil
	}
	return use, &contracts.ToolResultBlock{
		CallID: callID,
		Output: resultText,
	}
}

// decodeToolResult pulls the textual outcome out of a Copilot
// tool-invocation part. The shape varies between VS Code versions:
// some store the outcome as a single string, others as an array of
// {kind, value} parts that we have to flatten. We handle both.
func decodeToolResult(part map[string]any) string {
	if details, ok := part["resultDetails"].(map[string]any); ok {
		if text, ok := details["output"].(string); ok && text != "" {
			return text
		}
	}
	if outcome, ok := part["resultOutput"].(string); ok && outcome != "" {
		return outcome
	}
	if outcomeParts, ok := part["resultOutput"].([]any); ok {
		var combined string
		for _, raw := range outcomeParts {
			if outcomePart, ok := raw.(map[string]any); ok {
				if value := stringField(outcomePart, "value"); value != "" {
					combined += value
				}
			}
		}
		if combined != "" {
			return combined
		}
	}
	return ""
}

// epochMillisToTime turns a millisecond Unix timestamp (the format
// VS Code uses everywhere in this storage) into a Go time.Time.
// The zero value comes back for a zero or missing input, which the
// renderer recognizes and prints as "(unknown)" in the metadata
// blockquote.
func epochMillisToTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// stringField is a small helper that pulls a string out of a
// generic map. It returns the empty string if the key is missing
// or if the value is not a string. Almost every helper in this
// file ends up reaching for it.
func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

// nestedMap is the same idea as stringField but for a nested
// object. Returns nil when the key is missing or the value is not
// a map.
func nestedMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	v, _ := m[key].(map[string]any)
	return v
}
