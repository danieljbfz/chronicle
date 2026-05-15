// Package copilotchat is the chronicle adapter for the
// GitHub Copilot Chat extension in VS Code. Like the
// Claude adapter, its job is to translate the messy,
// tool-specific files on disk into the clean, shared
// types defined in the contracts package, so the rest of
// chronicle can render and operate on conversations
// without ever knowing how the extension's storage is
// laid out.
//
// VS Code's Copilot Chat extension keeps its data in two
// places under the user's VS Code config root:
//
//	workspaceStorage/<hash>/chatSessions/<sessionId>.jsonl
//	    one chat session, scoped to one workspace folder
//	workspaceStorage/<hash>/chatEditingSessions/<sessionId>/
//	    snapshots of files Copilot edited inside that session
//	workspaceStorage/<hash>/workspace.json
//	    maps the opaque workspace hash back to its folder URI
//	globalStorage/emptyWindowChatSessions/<sessionId>.jsonl
//	    chats from VS Code windows that were opened without a folder
//	globalStorage/github.copilot-chat/copilotCli/
//	    leftover CLI metadata from older Copilot versions
//
// Each session JSONL is an event log, not a stream of
// independent records like Claude's. The first line is a
// full snapshot of the session at the moment it was last
// saved. Every line after that is a tiny patch that mutates
// the snapshot in place. Reading a session means replaying
// every line so the current state can be reconstructed. The
// eventlog.go file does that work, and parse.go turns the
// reconstructed state into a contracts.Conversation.
//
// This adapter does NOT cover the GitHub Copilot agent
// runtime, which writes its own session state under
// ~/.copilot/session-state/ via the @github/copilot-sdk
// LocalSessionManager. That is a separate product with a
// completely different on-disk layout, modeled by the
// copilotagent adapter package as its own provider entry.
//
// Multiple roots. A single chronicle install often needs
// to read from more than one Copilot Chat root: the user
// might have both VS Code and VS Code Insiders installed.
// The factory in adapters/all.go produces one Entry per
// detected root, each with its own Provider value and its
// own cached storage version.
package copilotchat
