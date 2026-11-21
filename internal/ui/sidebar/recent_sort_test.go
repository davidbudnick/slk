package sidebar

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/cache"
)

// recentItems is a sidebar with direct messages, apps and a custom
// section, named so that alphabetical order differs from every other.
func recentItems() []ChannelItem {
	return []ChannelItem{
		{ID: "D1", Name: "ann", Type: "dm", DMUserID: "U1"},
		{ID: "D2", Name: "bob", Type: "dm", DMUserID: "U2"},
		{ID: "D3", Name: "cat", Type: "dm", DMUserID: "U3"},
		{ID: "D4", Name: "dan", Type: "dm", DMUserID: "U4"},
		{ID: "G1", Name: "eve, fay", Type: "group_dm"},
		{ID: "A1", Name: "jira", Type: "app", DMUserID: "UA1"},
		{ID: "A2", Name: "okta", Type: "app", DMUserID: "UA2"},
		{ID: "C1", Name: "alpha", Type: "channel", Section: "Team"},
		{ID: "C2", Name: "beta", Type: "channel", Section: "Team"},
	}
}

// order returns the names of the channel rows in the order they are drawn.
func order(m *Model) string {
	var names []string
	for _, n := range m.nav {
		if n.kind == navChannel {
			names = append(names, m.items[m.filtered[n.fi]].Name)
		}
	}
	return strings.Join(names, " | ")
}

// Slack lists direct messages and apps with the unread ones first, then
// by newest message; slk's own order, and every other section's, is by
// name.
func TestSlackStyle_DirectMessagesNewestFirstUnreadOnTop(t *testing.T) {
	state := map[string]cache.ReadState{
		"D1": {LatestTS: "1700000100.000000"},
		"D2": {LatestTS: "1700000400.000000"},
		"D3": {LatestTS: "1700000200.000000", HasUnread: true, MentionCount: 2},
		"D4": {},
		"G1": {LatestTS: "1700000300.000000"},
		"A1": {LatestTS: "1700000100.000000"},
		"A2": {LatestTS: "1700000900.000000", HasUnread: true, MentionCount: 1},
		"C1": {LatestTS: "1700000100.000000"},
		"C2": {LatestTS: "1700000900.000000", HasUnread: true},
	}
	m := New(recentItems())
	m.SetReadStateReader(func() map[string]cache.ReadState { return state })
	m.ToggleCollapse("Apps")

	const byName = "alpha | beta | ann | bob | cat | dan | eve, fay | jira | okta"
	if got := order(&m); got != byName {
		t.Fatalf("default order = %s\nwant by name: %s", got, byName)
	}

	m.SetSlackStyle(true)
	const recent = "alpha | beta | cat | bob | eve, fay | ann | dan | okta | jira"
	if got := order(&m); got != recent {
		t.Errorf("Slack style order = %s\nwant %s", got, recent)
	}

	// Slack keeps your DM with yourself at the bottom, however recent.
	m.SetSelfUserID("U2")
	const selfLast = "alpha | beta | cat | eve, fay | ann | dan | bob | okta | jira"
	if got := order(&m); got != selfLast {
		t.Errorf("order with bob as yourself = %s\nwant %s", got, selfLast)
	}
}

// A message arriving, or being read, moves its row without a rebuild of
// the list, and the cursor stays on the conversation it was on.
func TestSlackStyle_ReadStateChangeResortsUnderTheCursor(t *testing.T) {
	state := map[string]cache.ReadState{
		"D1": {LatestTS: "1700000100.000000"},
		"D2": {LatestTS: "1700000400.000000"},
		"D3": {LatestTS: "1700000200.000000"},
	}
	m := New(recentItems()[:3])
	m.SetReadStateReader(func() map[string]cache.ReadState { return state })
	m.SetSlackStyle(true)
	if got := order(&m); got != "bob | cat | ann" {
		t.Fatalf("order = %s, want newest first", got)
	}
	m.SelectByID("D2")

	state["D1"] = cache.ReadState{LatestTS: "1700000500.000000", HasUnread: true, MentionCount: 1}
	m.Invalidate()
	if got := order(&m); got != "ann | bob | cat" {
		t.Errorf("after ann's new message, order = %s, want ann first", got)
	}
	if got := m.SelectedID(); got != "D2" {
		t.Errorf("cursor moved to %q, want it still on bob (D2)", got)
	}

	state["D1"] = cache.ReadState{LatestTS: "1700000500.000000"}
	state["D3"] = cache.ReadState{LatestTS: "1700000200.000000", HasUnread: true, MentionCount: 3}
	m.Invalidate()
	if got := order(&m); got != "cat | ann | bob" {
		t.Errorf("after reading ann, order = %s, want unread cat first, then newest", got)
	}
}

// Slack's own section list keys its sections by ID; its direct-message
// and apps sections are the ones sorted, not a custom section of people.
func TestSlackStyle_RecentOrderFollowsSlacksSectionTypes(t *testing.T) {
	state := map[string]cache.ReadState{
		"D1": {LatestTS: "1700000100.000000"},
		"D2": {LatestTS: "1700000400.000000"},
		"D3": {LatestTS: "1700000900.000000"},
		"D4": {LatestTS: "1700000950.000000"},
		"A1": {LatestTS: "1700000100.000000"},
		"A2": {LatestTS: "1700000900.000000"},
	}
	items := recentItems()
	items[2].Section, items[3].Section = "TEAM", "TEAM"
	m := New(items)
	m.SetReadStateReader(func() map[string]cache.ReadState { return state })
	m.SetSectionsProvider(&fakeProvider{ready: true, sections: []SectionMeta{
		{ID: "TEAM", Name: "Team", Type: "standard"},
		{ID: "DMS", Name: "Direct messages", Type: "direct_messages"},
		{ID: "APPS", Name: "Recent apps", Type: "recent_apps"},
	}})
	m.SetSlackStyle(true)
	if got, want := order(&m), "cat | dan | bob | ann | eve, fay | okta | jira"; got != want {
		t.Errorf("order = %s\nwant %s", got, want)
	}
}
