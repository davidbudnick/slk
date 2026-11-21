package sidebar

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/cache"
)

const closeTestHeight, closeTestWidth = 20, 44

// closeSidebar is a Slack-style sidebar with a channel, two DMs (one
// with unread mentions) and a group DM.
func closeSidebar() Model {
	m := New([]ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
		{ID: "D1", Name: "Ann Lee", Type: "dm", DMUserID: "U1"},
		{ID: "G1", Name: "Bo Ito, Cy Day", Type: "group_dm", GroupSize: 2},
		{ID: "D2", Name: "Una Read", Type: "dm", DMUserID: "U2"},
	})
	m.SetSlackStyle(true)
	m.SetRightAlignBadges(true)
	m.SetReadStateReader(func() map[string]cache.ReadState {
		return map[string]cache.ReadState{"D2": {HasUnread: true, MentionCount: 2}}
	})
	return m
}

// lineOf returns the panel row that draws name.
func lineOf(t *testing.T, view, name string) int {
	t.Helper()
	for i, l := range strings.Split(view, "\n") {
		if strings.Contains(l, name) {
			return i
		}
	}
	t.Fatalf("row %q not rendered:\n%s", name, view)
	return -1
}

// Slack shows an "x" at the right end of the DM row the pointer is
// over. slk draws it there too, on that row alone, without moving
// anything else on the row.
func TestCloseMark_OnlyOnTheDMRowUnderThePointer(t *testing.T) {
	m := closeSidebar()
	plain := func(s string) string { return ansiRe.ReplaceAllString(s, "") }
	markCol := closeTestWidth - 2 - closeMarkInset + 1

	before := m.View(closeTestHeight, closeTestWidth)
	if strings.Contains(plain(before), "✕") {
		t.Fatalf("a close mark is drawn with the pointer nowhere:\n%s", plain(before))
	}

	for _, name := range []string{"Ann Lee", "Bo Ito, Cy Day"} {
		line := lineOf(t, before, name)
		m.SetHover(line)
		view := m.View(closeTestHeight, closeTestWidth)
		rows := strings.Split(view, "\n")
		if got := strings.Count(plain(view), "✕"); got != 1 {
			t.Fatalf("%s hovered: %d close marks drawn, want 1:\n%s", name, got, plain(view))
		}
		row := []rune(plain(rows[line]))
		if len(row) <= markCol || row[markCol] != '✕' {
			t.Errorf("%s hovered: mark not at column %d: %q", name, markCol, string(row))
		}
		if w := lipgloss.Width(rows[line]); w != closeTestWidth-2 {
			t.Errorf("%s hovered: row is %d cells wide, want %d", name, w, closeTestWidth-2)
		}
		if !strings.HasPrefix(strings.TrimLeft(string(row), " ▌"), strings.TrimLeft(plain(strings.Split(before, "\n")[line]), " ▌")[:len(name)+2]) {
			t.Errorf("%s hovered: the row's start moved: %q", name, string(row))
		}
	}

	m.ToggleCollapse("Channels")
	before = m.View(closeTestHeight, closeTestWidth)
	for _, name := range []string{"general", "Una Read", "Threads", "Direct Messages"} {
		m.SetHover(lineOf(t, before, name))
		if view := plain(m.View(closeTestHeight, closeTestWidth)); strings.Contains(view, "✕") {
			t.Errorf("%s hovered: a close mark is drawn on a row that cannot be closed by it:\n%s", name, view)
		}
	}

	m.SetHover(lineOf(t, before, "Ann Lee"))
	m.SetHover(-1)
	if view := plain(m.View(closeTestHeight, closeTestWidth)); strings.Contains(view, "✕") {
		t.Errorf("the mark stayed after the pointer left:\n%s", view)
	}
}

// The mark keeps the row's own colours: the cut that makes room for it
// must not leave it on the terminal's default background.
func TestCloseMark_KeepsTheRowsStyle(t *testing.T) {
	row := "\x1b[48;2;30;30;46m\x1b[38;2;200;200;200m" + strings.Repeat("a", 20) + "\x1b[0m"
	got := withCloseMark(row, 22)
	i := strings.Index(got, closeMark)
	if i < 0 {
		t.Fatalf("no mark in %q", got)
	}
	if lead := got[:i]; !strings.HasSuffix(lead, "\x1b[48;2;30;30;46m\x1b[38;2;200;200;200m") {
		t.Errorf("mark is not drawn in the row's style: %q", got)
	}
	if plain := ansiRe.ReplaceAllString(got, ""); plain != strings.Repeat("a", 16)+closeMark+"aa" {
		t.Errorf("cells either side of the mark changed: %q", plain)
	}
}

// A click on the mark closes; a click anywhere else on the row, or on
// the mark's cells of a row that does not show one, does not.
func TestCloseAt(t *testing.T) {
	m := closeSidebar()
	view := m.View(closeTestHeight, closeTestWidth)
	ann, una := lineOf(t, view, "Ann Lee"), lineOf(t, view, "Una Read")
	markCol := closeTestWidth - 2 - closeMarkInset + 1

	if _, ok := m.CloseAt(markCol, ann); ok {
		t.Error("CloseAt hit a mark that is not drawn (pointer not over the row)")
	}
	m.SetHover(ann)
	m.View(closeTestHeight, closeTestWidth)
	item, ok := m.CloseAt(markCol, ann)
	if !ok || item.ID != "D1" {
		t.Errorf("CloseAt on the mark = %q, %v; want D1", item.ID, ok)
	}
	if _, ok := m.CloseAt(8, ann); ok {
		t.Error("CloseAt hit on the row's name")
	}
	if _, ok := m.CloseAt(markCol, ann+1); ok {
		t.Error("CloseAt hit on the row below the hovered one")
	}
	m.SetHover(una)
	if _, ok := m.CloseAt(markCol, una); ok {
		t.Error("CloseAt hit on a row with an unread badge where the mark would be")
	}

	off := closeSidebar()
	off.SetSlackStyle(false)
	off.View(closeTestHeight, closeTestWidth)
	off.SetHover(ann)
	if off.TracksHover() {
		t.Error("TracksHover without the Slack look")
	}
	if strings.Contains(ansiRe.ReplaceAllString(off.View(closeTestHeight, closeTestWidth), ""), "✕") {
		t.Error("a close mark is drawn without the Slack look")
	}
}

func TestClosable(t *testing.T) {
	for typ, want := range map[string]bool{"dm": true, "group_dm": true, "app": true, "channel": false, "private": false} {
		if got := Closable(ChannelItem{Type: typ}); got != want {
			t.Errorf("Closable(%s) = %v; want %v", typ, got, want)
		}
	}
}
