package contracts

// StorageVersion is the result an adapter's Detect method returns. Every
// adapter must return a non-nil value for every call, including for
// storage shapes that no version of chronicle has ever recognized
// before. An unrecognized shape is a normal state, not an error. The
// resilience contract says we render unfamiliar storage in read-only
// mode and keep going. The Version field carries either the empty
// string or the literal "unknown" in that case, and the IsKnown
// helper below returns false.
type StorageVersion struct {
	Adapter      string       `json:"adapter"`
	Version      string       `json:"version"`
	Fingerprint  string       `json:"fingerprint"`
	Capabilities Capabilities `json:"capabilities"`
}

// Capabilities describes what an adapter understands about the
// storage it just looked at. The user interface checks these flags
// to decide which features to show. It does not look at the
// version string.
//
// Why? Because new versions of an upstream tool sometimes add a
// feature the existing adapter already knows how to handle. If the
// UI branched on the version string, we would have to ship a new
// chronicle release every time we added a fingerprint to the
// table. With capability flags, the fingerprint table can grow
// without changing the rest of the code.
type Capabilities struct {
	ThreadTree         bool `json:"thread_tree"`
	EditingSessions    bool `json:"editing_sessions"`
	ToolInvocations    bool `json:"tool_invocations"`
	ModelMetadata      bool `json:"model_metadata"`
	LiveWriterDetected bool `json:"live_writer_detected"`
}

// IsKnown reports whether the storage matched a recognized schema. The
// renderer uses this to decide whether to attach the "this session was
// written by an unrecognized version of the upstream tool" banner to
// the affected views, and the cleanup commands use it to require an
// extra confirmation step before doing anything destructive against
// unrecognized storage.
func (s StorageVersion) IsKnown() bool {
	return s.Version != "" && s.Version != "unknown"
}
