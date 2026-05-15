package composition

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/danieljbfz/chronicle/contracts"
)

// CleanCategory is the kind of cleanup the user wants to run.
// Each category corresponds to a single, well-defined heuristic
// for what counts as deletable: abandoned sessions are sessions
// with zero real user prompts; orphans are sibling files whose
// owning session no longer exists; and so on.
//
// Today only "abandoned" is implemented. Adding a new category
// (stale, large, by-project) is one new branch in PlanCleanup.
type CleanCategory string

const (
	// CategoryAbandoned finds sessions a user opened by accident
	// and never returned to. The IsAbandoned predicate on
	// Conversation defines the rule: zero real user prompts.
	CategoryAbandoned CleanCategory = "abandoned"
)

// PlannedDeletion pairs one DeletePlan with the providerEntry
// that produced it. The trash subsystem needs both pieces when
// it actually moves files. The plan tells it what to move, and
// the provider entry tells it which absolute root to combine
// with the plan's relative paths.
//
// We expose PlannedDeletion as a public type so the CLI can
// render dry-run output and then pass the exact same values
// back to ExecuteCleanup. The plan the user reviews on the
// screen is the plan we execute, with no possibility of drift
// in between.
type PlannedDeletion struct {
	provider *providerEntry
	Plan     contracts.DeletePlan
}

// ProviderName returns the name of the provider that produced
// the plan. The CLI uses this to label dry-run output.
func (pd PlannedDeletion) ProviderName() string {
	if pd.provider == nil {
		return ""
	}
	return pd.provider.Provider.Name()
}

// ProviderRoot returns the absolute filesystem root of the
// provider that produced the plan. Mostly useful for diagnostic
// output in dry-run mode, so the user can see exactly where the
// files live.
func (pd PlannedDeletion) ProviderRoot() string {
	if pd.provider == nil {
		return ""
	}
	return pd.provider.Root
}

// PlanCleanup walks every provider that supports cleanup and
// produces the deletion plans that would run if the user
// approved them. The result is a flat slice across providers,
// because the CLI renders the plans as one continuous stream.
//
// Pass an empty Categories slice to get every category at once.
// Pass a non-empty providerName to limit the result to one
// tool, like "claude" or "copilot".
//
// PlanCleanup is read-only. It walks the filesystem to find
// matching sessions, but it never moves or deletes anything.
// The caller passes the returned slice to ExecuteCleanup when
// the user confirms they want the cleanup applied.
func (a *App) PlanCleanup(categories []CleanCategory, providerName string) ([]PlannedDeletion, error) {
	if len(categories) == 0 {
		categories = []CleanCategory{CategoryAbandoned}
	}

	var planned []PlannedDeletion
	for _, p := range a.providers {
		if providerName != "" && p.Provider.Name() != providerName {
			continue
		}
		cleaner, ok := p.Provider.(contracts.Cleaner)
		if !ok {
			// Provider does not implement cleanup. Silently
			// skip; doctor view surfaces this if the user
			// asks.
			continue
		}
		for _, category := range categories {
			plans, err := planForCategory(p, cleaner, category)
			if err != nil {
				return nil, fmt.Errorf("composition.PlanCleanup: %s on %s: %w", category, p.Provider.Name(), err)
			}
			for _, plan := range plans {
				planned = append(planned, PlannedDeletion{provider: p, Plan: plan})
			}
		}
	}
	return planned, nil
}

// planForCategory dispatches to the right per-category builder.
// Keeping the dispatch in one function means that adding a new
// category in the future is one new case here plus one new
// helper below, with no other code changes.
func planForCategory(p *providerEntry, cleaner contracts.Cleaner, category CleanCategory) ([]contracts.DeletePlan, error) {
	switch category {
	case CategoryAbandoned:
		return planAbandonedSessions(p, cleaner)
	default:
		return nil, fmt.Errorf("unknown clean category %q", category)
	}
}

// planAbandonedSessions walks every project under the provider,
// reads each session, and produces a per-session DeletePlan for
// any session whose IsAbandoned predicate returns true. The
// definition of "abandoned" lives on the Conversation type, so
// future changes to that heuristic happen in one place and
// every category that uses it picks them up automatically.
//
// Reading every session is expensive on a busy install. We
// accept that cost because chronicle clean is an explicit
// user-triggered command, not a background task that runs at
// startup. The user sees the time it takes and is fine with it.
func planAbandonedSessions(p *providerEntry, cleaner contracts.Cleaner) ([]contracts.DeletePlan, error) {
	projects, err := p.Provider.ListProjects(p.FS)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var plans []contracts.DeletePlan
	for _, project := range projects {
		summaries, err := p.Provider.ListSessions(p.FS, project.ID)
		if err != nil {
			return nil, err
		}
		for _, summary := range summaries {
			conv, err := p.Provider.ReadSession(p.FS, summary.ID)
			if err != nil {
				return nil, err
			}
			if !conv.IsAbandoned() {
				continue
			}
			plan, err := cleaner.PlanDelete(p.FS, summary.ID)
			if err != nil {
				return nil, err
			}
			plans = append(plans, plan)
		}
	}
	return plans, nil
}

// ExecuteCleanup runs every planned deletion in order and
// returns one TrashEntry for each plan that moved successfully.
// If one of the plans fails partway through, the function
// returns the entries it has already created together with the
// error. The caller can then show the user how far the cleanup
// got before something went wrong.
//
// This is the function that actually changes the filesystem.
// Callers must not invoke it without explicit user approval.
// The CLI uses a separate --apply flag for exactly this reason:
// the default `chronicle clean` command stays a dry-run, and
// the user has to opt in to the destructive path.
func (a *App) ExecuteCleanup(planned []PlannedDeletion) ([]TrashEntry, error) {
	var entries []TrashEntry
	for _, pd := range planned {
		if len(pd.Plan.Items) == 0 {
			// Empty plan, nothing to move. Useful guard
			// because PlanCleanup may produce empty plans for
			// categories that found no targets.
			continue
		}
		entry, err := a.Trash(plannedDeletion{provider: pd.provider, plan: pd.Plan})
		if err != nil {
			return entries, fmt.Errorf("composition.ExecuteCleanup: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
