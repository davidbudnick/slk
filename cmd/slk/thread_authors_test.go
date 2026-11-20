package main

import (
	"fmt"
	"slices"
	"testing"

	"github.com/gammons/slk/internal/cache"
)

// queuedLookups returns the resolver's pending ids, sorted, and clears them.
func queuedLookups(r *userResolver) []string {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	var ids []string
	for id := range r.pending {
		ids = append(ids, id)
		delete(r.pending, id)
	}
	slices.Sort(ids)
	return ids
}

// summaryBy is a thread summary whose participants are ids.
func summaryBy(ids ...string) cache.ThreadSummary {
	var s cache.ThreadSummary
	copy(s.Participants[:], ids)
	return s
}

// The Threads view names every participant, so the ones slk has no name
// for are looked up; the list reloads constantly, so each is asked once.
func TestThreadAuthorRequests_AsksOnceForEachUnknownAuthor(t *testing.T) {
	db := newTestDB(t)
	wctx := &WorkspaceContext{TeamID: "T1", UserNames: newUserNameStore(map[string]string{"UKNOWN": "known"})}
	wctx.UserResolver = newUserResolver("T1", nil, db, nil, nil, &fakeBatcher{}, nil)
	summaries := []cache.ThreadSummary{summaryBy("UKNOWN", "UA"), summaryBy("UA", "UB")}

	var r threadAuthorRequests
	r.request(wctx, "T1", summaries)
	if got := queuedLookups(wctx.UserResolver); !slices.Equal(got, []string{"UA", "UB"}) {
		t.Fatalf("queued %v, want the two unknown authors", got)
	}
	wctx.UserResolver.inflight.Delete("UA")
	wctx.UserResolver.inflight.Delete("UB")
	r.request(wctx, "T1", summaries)
	if got := queuedLookups(wctx.UserResolver); len(got) != 0 {
		t.Errorf("a reload queued %v again, want nothing", got)
	}
	r.request(wctx, "T2", summaries)
	if got := queuedLookups(wctx.UserResolver); !slices.Equal(got, []string{"UA", "UB"}) {
		t.Errorf("another workspace queued %v, want its own lookups", got)
	}
}

func TestThreadAuthorRequests_OnlyTheNewestThreads(t *testing.T) {
	db := newTestDB(t)
	wctx := &WorkspaceContext{TeamID: "T1", UserNames: newUserNameStore(nil)}
	wctx.UserResolver = newUserResolver("T1", nil, db, nil, nil, &fakeBatcher{}, nil)
	summaries := make([]cache.ThreadSummary, threadAuthorLookupLimit+5)
	for i := range summaries {
		summaries[i] = summaryBy(fmt.Sprintf("U%04d", i))
	}

	var r threadAuthorRequests
	r.request(wctx, "T1", summaries)
	if got := len(queuedLookups(wctx.UserResolver)); got != threadAuthorLookupLimit {
		t.Errorf("queued %d lookups, want the newest %d threads' only", got, threadAuthorLookupLimit)
	}
	r.request(nil, "T1", summaries)
	r.request(&WorkspaceContext{}, "T1", summaries)
}
