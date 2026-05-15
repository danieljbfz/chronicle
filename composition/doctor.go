package composition

import "github.com/danieljbfz/chronicle/contracts"

// ProviderHealth is one row of the chronicle doctor view. It
// describes one provider's status at the moment Doctor was called.
// The Reachable field tells us whether listing succeeded. The
// SessionCount is the total number of sessions we found across all
// of that provider's projects. The Note is a human-readable line
// explaining anything odd, like "this storage version is unknown,
// destructive operations will require an extra confirmation".
type ProviderHealth struct {
	Name         string
	Root         string
	Version      contracts.StorageVersion
	Reachable    bool
	Note         string
	SessionCount int
}

// Doctor returns the current state of every wired provider. The
// chronicle doctor command renders the result as text, and the
// --json flag renders it as JSON for scripting.
//
// Doctor is read-only. It walks the providers, asks each one for
// its project list, and totals up the sessions. If a listing
// fails, the provider is marked not Reachable and the error
// message goes into Note. If the storage version is unknown, the
// Note explains that read-only operations still work but
// destructive ones will require an extra confirmation.
func (a *App) Doctor() []ProviderHealth {
	out := make([]ProviderHealth, 0, len(a.providers))
	for _, p := range a.providers {
		h := ProviderHealth{
			Name:    p.Provider.Name(),
			Root:    p.Root,
			Version: p.Version,
		}
		projects, err := p.Provider.ListProjects(p.FS)
		if err != nil {
			h.Reachable = false
			h.Note = err.Error()
		} else {
			h.Reachable = true
			for _, proj := range projects {
				h.SessionCount += proj.SessionCount
			}
			if !p.Version.IsKnown() {
				h.Note = "Storage version is unknown (fingerprint " + p.Version.Fingerprint + "). Read-only operations work. Destructive operations will require an extra confirmation."
			}
		}
		out = append(out, h)
	}
	return out
}
