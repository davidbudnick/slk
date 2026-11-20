package main

import "github.com/gammons/slk/internal/cache"

// threadAuthorLookupLimit caps how many of the newest threads have their
// authors resolved per list load, to bound users.info traffic on a
// workspace with a thousand subscriptions.
const threadAuthorLookupLimit = 100

// requestThreadAuthors queues a name lookup for every author the Threads
// view names in the newest threads that the name cache has not seen.
func requestThreadAuthors(wctx *WorkspaceContext, summaries []cache.ThreadSummary) {
	if wctx == nil || wctx.UserResolver == nil {
		return
	}
	for _, s := range summaries[:min(len(summaries), threadAuthorLookupLimit)] {
		for _, id := range s.ParticipantIDs() {
			if _, ok := wctx.UserNames.Get(id); !ok {
				wctx.UserResolver.Request(id)
			}
		}
	}
}
