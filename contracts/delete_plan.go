package contracts

// DeletePlan is the result of an adapter's PlanDelete or PlanOrphanScan.
// It describes every path that would move to trash if the plan is executed.
// Composition shows this to the user before any filesystem change.
type DeletePlan struct {
	SessionID SessionID // empty for orphan-scan plans
	Category  string    // e.g. "claude-session", "claude-orphan-file-history"
	Items     []DeleteItem
	SizeBytes int64    // sum of Items[].SizeBytes
	Warnings  []string // e.g. "VS Code is running", "unknown storage version"
}

// DeleteItem is one path within a DeletePlan.
type DeleteItem struct {
	Path      string
	Reason    string // "session file", "edit history", "orphan paste"
	SizeBytes int64
}
