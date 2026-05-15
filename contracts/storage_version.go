package contracts

// StorageVersion is the result of an adapter's Detect call. Every adapter
// returns a non-nil StorageVersion, including for unrecognized shapes —
// "unknown" is a normal state, not an error.
type StorageVersion struct {
	Adapter      string // matches Provider.Name(), e.g. "claude"
	Version      string // "claude-1.0", "copilot-3", or "unknown"
	Fingerprint  string // short hex hash from steps/fingerprint
	Capabilities Capabilities
}

// Capabilities advertises what an adapter understands about the storage at
// hand. UI features key off these, never off StorageVersion.Version.
type Capabilities struct {
	ThreadTree         bool // parentUuid graph (Claude) vs flat list (Copilot)
	EditingSessions    bool // sibling working-set storage exists
	ToolInvocations    bool // adapter recognizes tool calls in the model
	ModelMetadata      bool // storage records which model was used per turn
	LiveWriterDetected bool // an upstream process is actively writing here
}

// IsKnown reports whether the storage matched a recognized schema.
func (s StorageVersion) IsKnown() bool {
	return s.Version != "" && s.Version != "unknown"
}
