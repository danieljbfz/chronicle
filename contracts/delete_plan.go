package contracts

// DeletePlan is the result an adapter's PlanDelete or PlanOrphanScan
// method returns. The plan describes every path that would move to the
// trash if the user approved the plan, along with the total recoverable
// size and any warnings that should be shown before the user confirms.
// Composition shows the entire plan to the user before any filesystem
// change happens, so the user always knows what is about to disappear.
// Even an executed plan stays reversible until the trash is emptied,
// because deletion in chronicle always means "move to trash."
type DeletePlan struct {
	SessionID SessionID
	Category  string
	Items     []DeleteItem
	SizeBytes int64
	Warnings  []string
}

// DeleteItem is one path inside a DeletePlan. The Reason field carries
// a short human-readable explanation of why the path was included, like
// "session file" or "edit history" or "orphan paste cache entry," and
// the SizeBytes field is the on-disk size at the moment the plan was
// produced. The user interface displays both fields verbatim so the
// user can decide whether the plan looks right before approving it.
type DeleteItem struct {
	Path      string
	Reason    string
	SizeBytes int64
}
