package composition

import "github.com/danieljbfz/chronicle/contracts"

// ProviderHealth is one row of the chronicle doctor view. It
// describes one provider's status at the moment Doctor was
// called. The Reachable field tells us whether listing succeeded.
// SessionCount is the total number of sessions we found across
// every project the provider knows about.
//
// Errors and Warnings are separate slices on purpose. Errors mean
// "something is wrong and the user needs to do something about
// it" (e.g., a permission denial, a corrupted root directory).
// Warnings mean "we noticed something the user should know but
// chronicle still works" (e.g., unrecognized storage version, no
// sessions found). The text renderer indents them differently and
// the JSON output keeps them as separate arrays so scripts can
// branch on severity without parsing message text.
type ProviderHealth struct {
	Name         string                   `json:"name"`
	Root         string                   `json:"root"`
	Version      contracts.StorageVersion `json:"version"`
	Reachable    bool                     `json:"reachable"`
	SessionCount int                      `json:"session_count"`
	Errors       []string                 `json:"errors,omitempty"`
	Warnings     []string                 `json:"warnings,omitempty"`
}

// Doctor returns the current state of every wired provider. The
// chronicle doctor command renders the result as text, and the
// --json flag renders it as JSON for scripting.
//
// Doctor is read-only. It walks the providers, asks each one for
// its project list, and totals up the sessions. A failed listing
// becomes an entry in Errors with the message text. An
// unrecognized storage version becomes an entry in Warnings.
func (a *App) Doctor() []ProviderHealth {
	out := make([]ProviderHealth, 0, len(a.providers))
	for _, p := range a.providers {
		out = append(out, healthFor(p))
	}
	return out
}

// healthFor turns one providerEntry into its ProviderHealth row.
// We split the function out so the body of Doctor stays a clean
// "build a slice" loop and the per-provider work has its own
// place to grow.
func healthFor(p *providerEntry) ProviderHealth {
	h := ProviderHealth{
		Name:    p.Provider.Name(),
		Root:    p.Root,
		Version: p.Version,
	}

	projects, err := p.Provider.ListProjects(p.FS)
	if err != nil {
		h.Reachable = false
		h.Errors = append(h.Errors, err.Error())
		return h
	}
	h.Reachable = true
	for _, proj := range projects {
		h.SessionCount += proj.SessionCount
	}
	if !p.Version.IsKnown() {
		h.Warnings = append(h.Warnings,
			"Storage version is unknown (fingerprint "+p.Version.Fingerprint+
				"). Read-only operations work. Destructive operations will require an extra confirmation.")
	}
	return h
}
