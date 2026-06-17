package composition

import (
	"runtime"
	"sort"

	"golang.org/x/sync/errgroup"

	"github.com/danieljbfz/chronicle/contracts"
)

// summariesForProject returns one provider project's listing summaries,
// newest-first. It enumerates the sessions without parsing, serves the
// ones whose files are unchanged from the cache, and parses only the
// misses — in parallel. A session that fails to summarize is skipped, not
// fatal, so one unreadable or corrupt file never buries the rest of the
// listing — the resilient stance the search path and the copilot-agent
// adapter already take.
func (a *App) summariesForProject(p *providerEntry, project contracts.ProjectID) ([]contracts.SessionSummary, error) {
	// Step 1: enumerate the sessions without parsing them.
	refs, err := p.Provider.ListSessionRefs(p.FS, project)
	if err != nil {
		return nil, err
	}

	cache := a.summaryCacheHandle()
	name := p.Provider.Name()
	fingerprint := p.Version.Fingerprint

	// Step 2: give every session a result slot, fill the cache hits now,
	// and note which slots still need a parse. A nil slot is one that was
	// either a miss waiting to parse or a parse that failed; the collect
	// pass at the end skips whatever stayed nil.
	results := make([]*contracts.SessionSummary, len(refs))
	var misses []int
	for i, ref := range refs {
		if summary, ok := cache.get(name, fingerprint, ref); ok {
			results[i] = &summary
			continue
		}
		misses = append(misses, i)
	}

	// Step 3: parse the misses in parallel, bounded to the processor count
	// so a cold pass does not start hundreds of large parses at once. Each
	// goroutine fills its own slot, so the writes never overlap and need no
	// lock. A session that fails to parse leaves its slot nil and is
	// skipped, so one unreadable file never buries the rest of the listing.
	group := new(errgroup.Group)
	group.SetLimit(runtime.NumCPU())
	for _, i := range misses {
		group.Go(func() error {
			summary, err := p.Provider.SummarizeSession(p.FS, refs[i])
			if err != nil {
				return nil
			}
			cache.put(name, fingerprint, refs[i], summary)
			results[i] = &summary
			return nil
		})
	}
	_ = group.Wait()

	// Step 4: collect the filled slots and sort newest-first, the order
	// every listing surface expects.
	out := make([]contracts.SessionSummary, 0, len(results))
	for _, summary := range results {
		if summary != nil {
			out = append(out, *summary)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastActive.After(out[j].LastActive)
	})
	return out, nil
}
