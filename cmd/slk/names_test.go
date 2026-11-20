package main

import (
	"reflect"
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/slack/boot"
	"github.com/gammons/slk/internal/slack/edge"
	"github.com/gammons/slk/internal/ui"
	"github.com/slack-go/slack"
)

// withFullNames turns [appearance] full_names on for one test.
func withFullNames(t *testing.T) {
	t.Helper()
	fullNames.Store(true)
	t.Cleanup(func() { fullNames.Store(false) })
}

func TestShownName(t *testing.T) {
	if got := shownName("Ann Lee", "ann"); got != "ann" {
		t.Errorf("full names off: shownName = %q; want the display name ann", got)
	}
	withFullNames(t)
	if got := shownName("Ann Lee", "ann"); got != "Ann Lee" {
		t.Errorf("full names on: shownName = %q; want Ann Lee", got)
	}
	if got := shownName("", "ann"); got != "ann" {
		t.Errorf("full names on, none known: shownName = %q; want ann", got)
	}
}

func TestCachedUserName(t *testing.T) {
	u := cache.User{Name: "ann.lee", DisplayName: "ann", RealName: "Ann Lee"}
	if got := cachedUserName(u); got != "ann" {
		t.Errorf("full names off: cachedUserName = %q; want ann", got)
	}
	if got := cachedUserName(cache.User{Name: "ann.lee"}); got != "ann.lee" {
		t.Errorf("no display name: cachedUserName = %q; want the handle", got)
	}
	withFullNames(t)
	if got := cachedUserName(u); got != "Ann Lee" {
		t.Errorf("full names on: cachedUserName = %q; want Ann Lee", got)
	}
}

// Boot revalidation rewrites the DM peers' rows after the cache seed
// has read them; the names the sidebar is built from must be the
// rewritten ones, or the first start with full names on still shows
// display names.
func TestReseedPeerNames(t *testing.T) {
	withFullNames(t)
	db := newTestDB(t)
	wctx := &WorkspaceContext{
		UserNames:         newUserNameStore(nil),
		UserNamesByHandle: map[string]string{},
	}
	wctx.UserNames.Set("U1", "ann")
	wctx.UserNamesByHandle["ann.lee"] = "ann"
	if err := db.UpsertUser(cache.User{ID: "U1", WorkspaceID: "T1", Name: "ann.lee", DisplayName: "ann", RealName: "Ann Lee"}); err != nil {
		t.Fatal(err)
	}

	reseedPeerNames(wctx, db, []boot.IM{{ID: "D1", UserID: "U1"}, {ID: "D2", UserID: "U404"}})

	if got, _ := wctx.UserNames.Get("U1"); got != "Ann Lee" {
		t.Errorf("UserNames[U1] = %q; want Ann Lee", got)
	}
	if got := wctx.UserNamesByHandle["ann.lee"]; got != "Ann Lee" {
		t.Errorf("UserNamesByHandle[ann.lee] = %q; want Ann Lee", got)
	}
	if _, ok := wctx.UserNames.Get("U404"); ok {
		t.Error("a peer with no cached row was given a name")
	}
}

// A person cached before full names were stored is fetched once more
// when next asked for, and the full name is what the UI is told; a
// second ask in the same session does not fetch again.
func TestUserResolver_FetchesAMissingFullNameOnce(t *testing.T) {
	withFullNames(t)
	db := newTestDB(t)
	if err := db.UpsertUser(cache.User{ID: "U001", WorkspaceID: "T1", Name: "ann.lee", DisplayName: "ann"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertUser(cache.User{ID: "U002", WorkspaceID: "T1", Name: "appy", DisplayName: "appy", IsBot: true}); err != nil {
		t.Fatal(err)
	}
	batcher := &fakeBatcher{res: []edge.User{
		edgeUserRecord("U001", "ann.lee", "ann", "Ann Lee", "T1", 7, false),
	}}
	watch := newResolvedWatch(1, nil)
	r := newUserResolver("T1", nil, db, nil, watch.send, batcher, nil)
	r.names = newUserNameStore(nil)

	r.Request("U002")
	r.Request("U001")
	<-watch.done

	if calls := batcher.calls(); len(calls) != 1 || len(calls[0]) != 1 {
		t.Fatalf("edge batches = %v; want one, for U001 alone: an app has no full name to fetch", calls)
	}
	u, err := db.GetUser("U001")
	if err != nil {
		t.Fatal(err)
	}
	if u.RealName != "Ann Lee" || u.DisplayName != "ann" {
		t.Errorf("cached names = display %q, real %q; want ann and Ann Lee", u.DisplayName, u.RealName)
	}
	if got, _ := r.names.Get("U001"); got != "Ann Lee" {
		t.Errorf("name store = %q; want Ann Lee", got)
	}
	var told string
	for _, m := range watch.messages() {
		if rm, ok := m.(ui.UserResolvedMsg); ok && rm.UserID == "U001" {
			told = rm.DisplayName
		}
	}
	if told != "Ann Lee" {
		t.Errorf("UserResolvedMsg name = %q; want Ann Lee", told)
	}

	r.Request("U001")
	if u := (cache.User{ID: "U003", Name: "bo"}); !r.wantsRealName(u) || r.wantsRealName(u) {
		t.Error("wantsRealName must be true once per person per session, then false")
	}
	if calls := batcher.calls(); len(calls) != 1 {
		t.Errorf("edge batches after a second ask = %d; want still 1", len(calls))
	}
}

// groupWctx is a workspace whose user is grant, with names for ids.
func groupWctx(names map[string]string) *WorkspaceContext {
	wctx := &WorkspaceContext{
		UserID:            "UGRANT",
		UserHandle:        "grant",
		UserNames:         newUserNameStore(nil),
		UserNamesByHandle: map[string]string{"zed": "Zed Display"},
	}
	for id, name := range names {
		wctx.UserNames.Set(id, name)
	}
	return wctx
}

// mpdm is a group DM named for its handles, with the given members.
func mpdm(name string, members ...string) slack.Channel {
	return slack.Channel{GroupConversation: slack.GroupConversation{
		Conversation: slack.Conversation{ID: "G1", IsMpIM: true},
		Name:         name,
		Members:      members,
	}}
}

func TestGroupName(t *testing.T) {
	const raw = "mpdm-grant--zed--amy-1"
	named := map[string]string{"UZED": "Zed Young", "UAMY": "amy Okafor"}
	tests := []struct {
		name string
		wctx *WorkspaceContext
		ch   slack.Channel
		want string
	}{
		{"members named: alphabetical, self left out", groupWctx(named), mpdm(raw, "UZED", "UGRANT", "UAMY"), "amy Okafor, Zed Young"},
		{"a member without a name: the handles", groupWctx(map[string]string{"UZED": "Zed Young"}), mpdm(raw, "UZED", "UGRANT", "UAMY"), "Zed Display, amy"},
		{"no member list: the handles", groupWctx(named), mpdm(raw), "Zed Display, amy"},
		{"a custom name is kept", groupWctx(named), mpdm("Launch crew", "UZED", "UGRANT", "UAMY"), "Launch crew"},
	}
	for _, tt := range tests {
		if got := groupName(tt.ch, tt.wctx); got != tt.want {
			t.Errorf("%s: groupName = %q; want %q", tt.name, got, tt.want)
		}
	}
}

// The members slk cannot name, and with full names on those cached
// without one, are fetched in a single batch before the sidebar is
// built; a group's own user, apps and people already complete are not.
func TestResolveGroupMembers(t *testing.T) {
	withFullNames(t)
	db := newTestDB(t)
	for _, u := range []cache.User{
		{ID: "UFULL", WorkspaceID: "T1", Name: "full", DisplayName: "full", RealName: "Full Name"},
		{ID: "UOLD", WorkspaceID: "T1", Name: "old", DisplayName: "old"},
		{ID: "UAPP", WorkspaceID: "T1", Name: "appy", DisplayName: "appy", IsBot: true},
	} {
		if err := db.UpsertUser(u); err != nil {
			t.Fatal(err)
		}
	}
	batcher := &fakeBatcher{res: []edge.User{
		edgeUserRecord("UOLD", "old", "old", "Old Name", "T1", 3, false),
		edgeUserRecord("UNEW", "new", "", "New Name", "T1", 4, false),
	}}
	wctx := groupWctx(nil)
	wctx.UserResolver = newUserResolver("T1", nil, db, nil, nil, batcher, nil)
	wctx.UserResolver.names = wctx.UserNames

	resolveGroupMembers(wctx, db, []slack.Channel{
		mpdm("mpdm-grant--old--new-1", "UGRANT", "UOLD", "UNEW", "UFULL", "UAPP"),
		mpdm("mpdm-grant--old-1", "UGRANT", "UOLD"),
		{GroupConversation: slack.GroupConversation{Name: "general", Members: []string{"UCHANNEL"}}},
	})

	calls := batcher.calls()
	if len(calls) != 1 {
		t.Fatalf("edge batches = %d; want 1", len(calls))
	}
	if want := (map[string]int64{"UOLD": 0, "UNEW": 0}); !reflect.DeepEqual(calls[0], want) {
		t.Errorf("batch = %v; want %v", calls[0], want)
	}
	if got := groupName(mpdm("mpdm-grant--old--new-1", "UGRANT", "UOLD", "UNEW"), wctx); got != "New Name, Old Name" {
		t.Errorf("group name after the batch = %q; want New Name, Old Name", got)
	}
}
