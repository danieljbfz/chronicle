// Package copilotagent is the chronicle adapter for the
// GitHub Copilot agent runtime, the @github/copilot-sdk
// LocalSessionManager that persists session state under
// ~/.copilot/. The runtime can be invoked from several
// frontends: the standalone Copilot CLI, VS Code's agent
// mode, or any application that imports the SDK directly.
// All of them write to the same on-disk layout, so this
// adapter speaks one storage format regardless of which
// frontend produced the session.
//
// This adapter is distinct from the copilotchat adapter,
// which reads the legacy VS Code Copilot Chat extension's
// data under workspaceStorage. The two are different
// products under the GitHub Copilot brand: chat is the
// in-IDE conversational panel, agent is the autonomous
// runtime. They have non-overlapping data on disk.
//
// The agent format is one directory per session under
// ~/.copilot/session-state/<sessionId>/. Inside each
// directory:
//
//	events.jsonl                  the event stream that drives the session
//	vscode.metadata.json          present when VS Code launched the session;
//	                              carries the workspace folder and a custom title
//	vscode.requests.metadata.json request-id mappings (not yet read)
//	workspace.yaml                the agent's workspace snapshot at start
//	checkpoints/                  per-turn file checkpoints (not yet read)
//	files/                        file attachments (not yet read)
//	research/                     research artifacts (not yet read)
//
// events.jsonl uses a typed event envelope. Every line is
// one event with a "type" field, a "data" payload whose
// shape depends on the type, plus envelope fields like
// timestamp, id, and parentId. The known types are:
//
//	session.start              the session's metadata and cwd
//	user.message               one user turn, content and attachments
//	assistant.turn_start       opens an assistant turn
//	assistant.message          one assistant message inside the turn
//	tool.execution_start       a tool call the assistant requested
//	tool.execution_complete    the result of that tool call
//	assistant.turn_end         closes the assistant turn
//	session.shutdown           the session ended cleanly
//
// Reading a session means walking the events in order and
// folding them into a contracts.Conversation. The parse
// rules in parse.go handle each type. Unknown types
// produce an UnknownBlock instead of being dropped, the
// same resilience contract every adapter follows.
//
// The adapter is read-only at first. Cleanup, resume, and
// other capabilities follow once the read path is verified
// against real data.
package copilotagent
