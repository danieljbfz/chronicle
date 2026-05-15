// Package copilot is the chronicle adapter for VS Code's GitHub
// Copilot Chat. Like the Claude adapter, its job is to translate
// the messy, tool-specific files on disk into the clean, shared
// types defined in the contracts package, so the rest of chronicle
// can render and operate on conversations without ever knowing how
// Copilot's storage is laid out.
//
// VS Code keeps Copilot data in two places:
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
//	    Copilot CLI session metadata
//
// Each session JSONL is an event log, not a stream of independent
// records like Claude's. The first line is a full snapshot of the
// session at the moment it was last saved. Every line after that is
// a tiny patch that mutates the snapshot in place. Reading a
// session means replaying every line in order to reconstruct the
// current state. The eventlog.go file does that work, and parse.go
// turns the reconstructed state into a contracts.Conversation.
//
// The package today is read-only. Detect, ListProjects, ListSessions,
// and ReadSession all do real work. PlanDelete and PlanOrphanScan
// return ErrNotImplemented and the cleanup work will fill them in
// once the trash subsystem is ready.
//
// Multiple roots. A single chronicle install often needs to read
// from more than one Copilot root: the user might have both VS Code
// and VS Code Insiders installed. Each root becomes its own Entry
// in the registry, and each Entry has its own Provider value with
// its own cached storage version.
package copilot
