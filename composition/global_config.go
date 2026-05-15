package composition

import (
	"errors"
	"fmt"

	"github.com/danieljbfz/chronicle/contracts"
)

// ConfigProjectListing pairs one config-project entry with
// the provider that owns it. The CLI uses the provider name
// in dry-run output and to scope the apply step. We wrap
// the contract type rather than re-export it so the CLI has
// one stable presentation type and the provider stays free
// to evolve its own internal shape.
type ConfigProjectListing struct {
	Provider string
	Entry    contracts.ConfigProjectEntry
}

// ListConfigProjects walks every provider that implements
// the optional GlobalConfig capability and returns one
// listing per per-project config entry. Pass a non-empty
// providerName to limit the result to one tool. The
// listing covers both stale entries (Exists == false) and
// live ones (Exists == true), so the CLI can present
// either view and the user can spot-check before requesting
// a cleanup.
//
// Providers that do not implement GlobalConfig contribute
// nothing. The result is empty (not an error) when no
// provider has a global config file, which is the normal
// state for an install with the relevant tool absent.
func (a *App) ListConfigProjects(providerName string) ([]ConfigProjectListing, error) {
	var out []ConfigProjectListing
	for _, p := range a.providers {
		if providerName != "" && p.Provider.Name() != providerName {
			continue
		}
		store, ok := p.Provider.(contracts.GlobalConfig)
		if !ok {
			continue
		}
		entries, err := store.ListConfigProjectEntries(p.FS)
		if err != nil {
			return nil, fmt.Errorf("config projects: %s: %w", p.Provider.Name(), err)
		}
		for _, entry := range entries {
			out = append(out, ConfigProjectListing{
				Provider: p.Provider.Name(),
				Entry:    entry,
			})
		}
	}
	return out, nil
}

// CleanConfigProjects removes the stale per-project
// entries from each provider's global config file. The
// caller has already chosen which entries to remove
// (presumably by filtering the result of ListConfigProjects
// for Exists == false), so this method does not reapply
// the staleness check. That keeps the dry-run plan and
// the apply step from drifting if the filesystem changes
// between the two.
//
// The function returns one ConfigCleanupResult per
// provider that contributed a removal, capturing the
// backup path so the caller can report where to recover
// from. Providers with nothing to remove are skipped from
// the result.
func (a *App) CleanConfigProjects(toRemove []ConfigProjectListing) ([]ConfigCleanupResult, error) {
	if len(toRemove) == 0 {
		return nil, nil
	}

	// Step 1: bucket the requested removals by provider.
	// We need one call per provider regardless of how many
	// keys it owns, because the atomic-write semantics
	// belong to the provider's own file.
	byProvider := map[string][]string{}
	for _, l := range toRemove {
		byProvider[l.Provider] = append(byProvider[l.Provider], l.Entry.Key)
	}

	// Step 2: dispatch each bucket to the right provider.
	// Order is the registration order of providers, so the
	// CLI output is stable across runs.
	var results []ConfigCleanupResult
	for _, p := range a.providers {
		keys, ok := byProvider[p.Provider.Name()]
		if !ok {
			continue
		}
		store, ok := p.Provider.(contracts.GlobalConfig)
		if !ok {
			return results, fmt.Errorf("config projects: provider %s does not support global config edits",
				p.Provider.Name())
		}
		backup, err := store.RemoveConfigProjectEntries(p.FS, keys)
		if err != nil {
			return results, fmt.Errorf("config projects: %s: %w", p.Provider.Name(), err)
		}
		results = append(results, ConfigCleanupResult{
			Provider:    p.Provider.Name(),
			RemovedKeys: keys,
			BackupPath:  backup,
		})
	}
	return results, nil
}

// ConfigCleanupResult captures what one provider removed
// from its global config file. The CLI uses these to tell
// the user how many entries went and where to find the
// backup.
type ConfigCleanupResult struct {
	Provider    string
	RemovedKeys []string
	BackupPath  string
}

// ErrNoGlobalConfigCapability is the explicit error
// CleanConfigProjects returns when the caller asked to
// clean entries from a provider that does not implement
// the GlobalConfig capability. The CLI surfaces this with
// a clear "this provider has no global config" message.
var ErrNoGlobalConfigCapability = errors.New("provider does not support global config edits")
