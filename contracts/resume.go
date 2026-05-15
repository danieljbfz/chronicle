package contracts

import "io/fs"

// Resumable is the optional capability adapters implement
// when their tool can re-open a stored session from the
// command line. Today Claude Code is the only adapter that
// qualifies. Copilot Chat lives inside VS Code and has no
// external API to jump to a specific chat by identifier, so
// the Copilot adapter deliberately does not implement this
// interface. When the user asks chronicle to resume a
// Copilot session, composition reports that the provider
// does not support resume rather than silently doing
// nothing or guessing at a workaround.
//
// Composition discovers Resumable the same way it finds the
// other capability interfaces (Cleaner, MemoryStore): with
// a type assertion at the call site. Keeping the surface
// optional means a new adapter can land with read-only
// support today and add resume later without ever having
// to ship a stub method that pretends to work.
type Resumable interface {
	// ResumeCommand returns the executable, the arguments,
	// and the working directory chronicle uses to reopen
	// one session. The function returns an error wrapping
	// fs.ErrNotExist when the session identifier is not
	// known to this provider, which lets composition chain
	// errors.Is the same way it does for ReadSession.
	//
	// Implementations should not perform the launch
	// themselves. The contract is "tell the caller what to
	// run," not "run it." That separation keeps every
	// process-spawning concern (stdio attachment, signal
	// forwarding, exit code propagation) inside the CLI
	// layer, where it can be tested with the standard
	// os/exec patterns rather than hidden behind an
	// adapter-specific shim.
	ResumeCommand(root fs.FS, id SessionID) (ResumePlan, error)
}

// ResumePlan describes how to relaunch one session. The
// shape is small on purpose. Chronicle does not try to
// model every flag the underlying tool might accept. It
// captures the irreducible bits (which executable, with
// which arguments, in which directory) and lets the
// upstream tool handle everything else through its own
// configuration.
type ResumePlan struct {
	// Command is the full argv slice. Command[0] is the
	// executable name, which the CLI looks up through PATH
	// at exec time. Subsequent entries are the arguments
	// the underlying tool needs to find the session, like
	// "--resume" followed by the session UUID.
	Command []string

	// WorkingDir is the absolute path chronicle chdirs to
	// before exec. The directory matters because some
	// tools key their session storage by working directory.
	// Claude Code, for example, looks up sessions in
	// ~/.claude/projects/<encoded-cwd>/, so launching from
	// the wrong directory would silently fail to find the
	// session even though the executable runs cleanly.
	WorkingDir string
}
