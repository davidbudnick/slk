package main

import (
	"sync"

	"github.com/gammons/slk/internal/cache"
)

// threadAuthorLookupLimit caps how many of the newest threads have their
// authors resolved per list load, to bound users.info traffic on a
// workspace with a thousand subscriptions.
const threadAuthorLookupLimit = 100

// threadAuthorRequests remembers the authors already queued for a name
// lookup. The Threads list reloads on every reply and mark-read, and the
// resolver keeps no record of a lookup that failed, so without this an
// ID Slack cannot resolve would be asked for again on every reload.
type threadAuthorRequests struct {
	mu    sync.Mutex
	asked map[string]struct{}
}

// request queues a name lookup for every author the Threads view names
// in the newest threads that the name cache has not seen, once per run.
func (r *threadAuthorRequests) request(wctx *WorkspaceContext, teamID string, summaries []cache.ThreadSummary) {
	if wctx == nil || wctx.UserResolver == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.asked == nil {
		r.asked = map[string]struct{}{}
	}
	for _, s := range summaries[:min(len(summaries), threadAuthorLookupLimit)] {
		for _, id := range s.ParticipantIDs() {
			if _, known := wctx.UserNames.Get(id); known {
				continue
			}
			key := teamID + "/" + id
			if _, done := r.asked[key]; done {
				continue
			}
			r.asked[key] = struct{}{}
			wctx.UserResolver.Request(id)
		}
	}
}
