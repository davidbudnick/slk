package threadsview

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/cache"
)

func sampleSummaries() []cache.ThreadSummary {
	return []cache.ThreadSummary{
		{
			ChannelID: "C1", ChannelName: "general", ChannelType: "channel",
			ThreadTS: "1.000000", ParentUserID: "U1", ParentText: "hello world",
			ParentTS: "1.000000", ReplyCount: 3, LastReplyTS: "5.000000", LastReplyBy: "U2",
			Unread: true,
		},
		{
			ChannelID: "C2", ChannelName: "design", ChannelType: "channel",
			ThreadTS: "2.000000", ParentUserID: "U2", ParentText: "spec review",
			ParentTS: "2.000000", ReplyCount: 1, LastReplyTS: "4.000000", LastReplyBy: "USELF",
			Unread: false,
		},
	}
}

func TestNew_StartsAtTop(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries(sampleSummaries())
	if got := m.SelectedIndex(); got != 0 {
		t.Errorf("SelectedIndex = %d, want 0", got)
	}
}

func TestMoveDown_ClampsAtBottom(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries(sampleSummaries())
	m.MoveDown()
	if m.SelectedIndex() != 1 {
		t.Errorf("after MoveDown SelectedIndex = %d, want 1", m.SelectedIndex())
	}
	m.MoveDown()
	if m.SelectedIndex() != 1 {
		t.Errorf("MoveDown past end should clamp; got %d, want 1", m.SelectedIndex())
	}
}

func TestSelected_ReturnsChannelAndThread(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries(sampleSummaries())
	m.MoveDown()
	chID, threadTS, ok := m.Selected()
	if !ok || chID != "C2" || threadTS != "2.000000" {
		t.Errorf("Selected = (%q, %q, %v); want (C2, 2.000000, true)", chID, threadTS, ok)
	}
}

func TestSetSummaries_PreservesSelectionByThreadTS(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries(sampleSummaries())
	m.MoveDown() // selected: thread 2

	// Re-rank: thread 2 moves to position 0, thread 1 to position 1.
	reranked := []cache.ThreadSummary{sampleSummaries()[1], sampleSummaries()[0]}
	m.SetSummaries(reranked)

	if m.SelectedIndex() != 0 {
		t.Errorf("after re-rank SelectedIndex should follow thread 2 to index 0, got %d", m.SelectedIndex())
	}
	chID, _, _ := m.Selected()
	if chID != "C2" {
		t.Errorf("Selected channel should still be C2, got %s", chID)
	}
}

func TestVersion_BumpsOnMutation(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	v0 := m.Version()
	m.SetSummaries(sampleSummaries())
	v1 := m.Version()
	if v1 == v0 {
		t.Errorf("Version did not bump on SetSummaries (v0=%d v1=%d)", v0, v1)
	}
	m.MoveDown()
	v2 := m.Version()
	if v2 == v1 {
		t.Errorf("Version did not bump on MoveDown")
	}
}

func TestUnreadCount(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries(sampleSummaries())
	if got := m.UnreadCount(); got != 1 {
		t.Errorf("UnreadCount = %d, want 1", got)
	}
}

// M7: Parent-not-loaded fallback renders the placeholder when both
func TestSetUserNames_IdempotentDoesNotBumpVersion(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	names := map[string]string{"U1": "alice", "U2": "bob"}
	m.SetUserNames(names)
	v0 := m.Version()
	m.SetUserNames(names) // same map -> should be a no-op
	if v1 := m.Version(); v1 != v0 {
		t.Errorf("SetUserNames(same map) bumped Version: v0=%d v1=%d", v0, v1)
	}
	// A genuinely different map MUST still bump.
	m.SetUserNames(map[string]string{"U1": "alice", "U2": "carol"})
	if v2 := m.Version(); v2 == v0 {
		t.Errorf("SetUserNames(different map) did NOT bump Version: v0=%d v2=%d", v0, v2)
	}
}

// TestVersion_StableAcrossIdenticalSetCalls is the regression guard for the
// app.go:4068-4093 cache-key bug: pushing the same userNames + selfUserID
// repeatedly must NOT bump Version, otherwise the panel cache can never hit.
func TestVersion_StableAcrossIdenticalSetCalls(t *testing.T) {
	names := map[string]string{"U1": "alice", "U2": "bob"}
	m := New(names, "U1")
	m.SetSummaries(sampleSummaries())
	v0 := m.Version()
	for i := 0; i < 5; i++ {
		m.SetUserNames(names)
		m.SetSelfUserID("U1")
	}
	if v1 := m.Version(); v1 != v0 {
		t.Errorf("Version drifted across identical Set calls: v0=%d v1=%d", v0, v1)
	}
}

func TestMarkByThreadTSUnread_FlipsFlagAndReturnsTrue(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries([]cache.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "P1", Unread: false},
		{ChannelID: "C1", ThreadTS: "P2", Unread: false},
	})

	if !m.MarkByThreadTSUnread("C1", "P2") {
		t.Fatal("expected return true when flag flipped")
	}

	for _, s := range m.Summaries() {
		if s.ThreadTS == "P2" && !s.Unread {
			t.Error("P2 should be Unread=true after MarkByThreadTSUnread")
		}
		if s.ThreadTS == "P1" && s.Unread {
			t.Error("P1 should remain Unread=false")
		}
	}
}

func TestMarkByThreadTSUnread_AlreadyUnread_ReturnsFalse(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries([]cache.ThreadSummary{{ChannelID: "C1", ThreadTS: "P1", Unread: true}})

	if m.MarkByThreadTSUnread("C1", "P1") {
		t.Error("expected false when flag was already true")
	}
}

func TestMarkByThreadTSUnread_NotFound_ReturnsFalse(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries([]cache.ThreadSummary{{ChannelID: "C1", ThreadTS: "P1", Unread: false}})

	if m.MarkByThreadTSUnread("C2", "P9") {
		t.Error("expected false when (channel, thread) not in summaries")
	}
}

func TestMarkByThreadTSUnread_EmptyArgs_ReturnsFalse(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries([]cache.ThreadSummary{{ChannelID: "C1", ThreadTS: "P1", Unread: false}})

	if m.MarkByThreadTSUnread("", "P1") {
		t.Error("expected false for empty channelID")
	}
	if m.MarkByThreadTSUnread("C1", "") {
		t.Error("expected false for empty threadTS")
	}
}

func TestMarkByThreadTSReadAt_CursorBehindLatestSetsUnread(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries([]cache.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "P1", LastReplyTS: "5.000000", Unread: false},
	})

	if !m.MarkByThreadTSReadAt("C1", "P1", "4.000000") {
		t.Fatal("want true when the flag flips to unread")
	}
	if !m.Summaries()[0].Unread {
		t.Error("cursor behind latest reply must render unread")
	}
}

func TestMarkByThreadTSReadAt_CursorAtLatestClearsUnread(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries([]cache.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "P1", LastReplyTS: "5.000000", Unread: true},
	})

	if !m.MarkByThreadTSReadAt("C1", "P1", "5.000000") {
		t.Fatal("want true when the flag flips to read")
	}
	if m.Summaries()[0].Unread {
		t.Error("cursor at latest reply must render read")
	}
}

func TestMarkByThreadTSReadAt_NoChangeReturnsFalse(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries([]cache.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "P1", LastReplyTS: "5.000000", Unread: true},
	})

	if m.MarkByThreadTSReadAt("C1", "P1", "4.000000") {
		t.Error("want false when the flag is already correct")
	}
}

func TestMarkByThreadTSReadAt_UnknownThreadReturnsFalse(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries([]cache.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "P1", LastReplyTS: "5.000000", Unread: true},
	})

	if m.MarkByThreadTSReadAt("C1", "P_MISSING", "9.000000") {
		t.Error("want false for a thread not in the list")
	}
	if m.MarkByThreadTSReadAt("", "P1", "9.000000") {
		t.Error("want false for an empty channelID")
	}
}

func TestMarkByThreadTSReadAt_BumpsVersionOnlyOnChange(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries([]cache.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "P1", LastReplyTS: "5.000000", Unread: false},
	})

	before := m.Version()
	if !m.MarkByThreadTSReadAt("C1", "P1", "4.000000") {
		t.Fatal("want true when the flag flips to unread")
	}
	if m.Version() == before {
		t.Error("a changed flag must bump the version so the list re-renders")
	}

	after := m.Version()
	if m.MarkByThreadTSReadAt("C1", "P1", "4.000000") {
		t.Fatal("want false on the repeated no-op call")
	}
	if m.Version() != after {
		t.Error("a no-op must not bump the version")
	}
}

// PatchUserName delivers a name resolved after SetUserNames. The map is
// usually the App's shared one, which already holds the name, so the
// version must bump on the user being referenced, not on a map change.
func TestPatchUserName_BumpsVersionOnlyWhenListReferencesUser(t *testing.T) {
	shared := map[string]string{}
	m := New(shared, "USELF")
	m.SetSummaries([]cache.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "1.0", ParentUserID: "U1", LastReplyBy: "U2", ParentText: "hi"},
		{ChannelID: "C2", ThreadTS: "2.0", ParentUserID: "U2", LastReplyBy: "U2", ParentText: "ping <@U3>"},
	})
	for _, c := range []struct {
		uid  string
		want bool
	}{{"U1", true}, {"U2", true}, {"U3", true}, {"U4", false}, {"", false}} {
		shared[c.uid] = "name-" + c.uid // the App writes the shared map first
		v0 := m.Version()
		m.PatchUserName(c.uid, "name-"+c.uid)
		if bumped := m.Version() != v0; bumped != c.want {
			t.Errorf("PatchUserName(%q) bumped=%v, want %v", c.uid, bumped, c.want)
		}
	}
	if !strings.Contains(m.View(20, 60), "name-U1") {
		t.Error("View does not show the patched name")
	}
}

// Without a shared map (SetUserNames never called) the name must still
// land in the model's own map.
func TestPatchUserName_RecordsNameInOwnMap(t *testing.T) {
	m := New(nil, "USELF")
	m.SetSummaries([]cache.ThreadSummary{{ChannelID: "C1", ThreadTS: "1.0", ParentUserID: "U1", ParentText: "hi"}})
	m.PatchUserName("U1", "alice")
	if !strings.Contains(m.View(20, 60), "alice") {
		t.Error("View does not show the patched name")
	}
}
