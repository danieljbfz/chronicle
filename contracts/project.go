package contracts

// Project groups the sessions that belong to one working directory or
// workspace from the upstream tool's point of view. The DisplayName is
// the human-readable project name the user sees in the listing, and the
// Path is the absolute filesystem path the project was decoded from
// when that information is available. SessionCount and SizeBytes are
// computed at listing time so the user interface can show useful
// summary information without loading every session.
type Project struct {
	ID           ProjectID
	DisplayName  string
	Path         string
	SessionCount int
	SizeBytes    int64
}
