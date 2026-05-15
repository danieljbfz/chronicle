package composition

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/danieljbfz/chronicle/contracts"
)

// ResumeResult wraps the contract's ResumePlan with the
// owning provider's name. The CLI uses the name in user
// messages ("resuming claude session ...") so the user
// always sees which underlying tool chronicle is about to
// launch.
type ResumeResult struct {
	Provider string
	Plan     contracts.ResumePlan
}

// ErrResumeUnsupported is returned by Resume when the
// session was found but its owning provider does not
// implement the Resumable capability. The CLI surfaces this
// as a clear "this provider cannot be resumed from outside"
// message rather than the generic not-found error, so the
// user knows the session exists and where it lives.
var ErrResumeUnsupported = errors.New("provider does not support resume")

// Resume locates the session, asks its owning provider for
// a launch plan, and returns it. The function never
// executes the plan. It only describes what to run, leaving
// the exec, stdio attachment, and signal forwarding to the
// CLI layer where those concerns belong.
//
// The lookup walks providers in registration order and
// returns the first match. Session identifiers are UUIDs,
// so collision across providers is astronomically unlikely
// in practice. We still walk in a defined order so the
// behaviour stays deterministic even if a future fixture
// happens to engineer a collision.
//
// Errors:
//   - errors.Is(err, fs.ErrNotExist) when no provider
//     recognises the session identifier.
//   - errors.Is(err, ErrResumeUnsupported) when a provider
//     owns the session but does not implement Resumable.
//     The wrapped error includes the provider name so the
//     CLI can name it in the user message.
func (a *App) Resume(id contracts.SessionID) (ResumeResult, error) {
	for _, p := range a.providers {
		// ReadSession is the existence probe today. It is
		// heavier than strictly needed (it parses the full
		// session) but resume is a one-shot interactive
		// action, so the cost is invisible to the user. If
		// future benchmarks show it matters, the right
		// move is a lightweight HasSession method on
		// Provider, not a special-case fast path here.
		if _, err := p.Provider.ReadSession(p.FS, id); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return ResumeResult{}, fmt.Errorf("resume %s: %w", p.Provider.Name(), err)
		}

		resumable, ok := p.Provider.(contracts.Resumable)
		if !ok {
			return ResumeResult{}, fmt.Errorf("resume %s: %w", p.Provider.Name(), ErrResumeUnsupported)
		}

		plan, err := resumable.ResumeCommand(p.FS, id)
		if err != nil {
			return ResumeResult{}, fmt.Errorf("resume %s: %w", p.Provider.Name(), err)
		}
		return ResumeResult{Provider: p.Provider.Name(), Plan: plan}, nil
	}
	return ResumeResult{}, fmt.Errorf("resume: %w", fs.ErrNotExist)
}
