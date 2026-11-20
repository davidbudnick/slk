package channelfinder

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/core"
)

// unreadFinder is a finder of four joined conversations, visited in the
// order ann, bob, cat, dan (dan last), with cat and ann unread: cat with
// two mentions, ann with a newer message and no count.
func unreadFinder() Model {
	m := New()
	m.SetItems([]core.ChannelFinderItem{
		{ID: "D1", Name: "ann", Type: "dm", Joined: true, LastVisited: 100},
		{ID: "D2", Name: "bob", Type: "dm", Joined: true, LastVisited: 200},
		{ID: "D3", Name: "cat", Type: "dm", Joined: true, LastVisited: 300},
		{ID: "C1", Name: "dan-channel", Type: "channel", Joined: true, LastVisited: 400},
	})
	m.SetUnreads(map[string]core.UnreadConversation{
		"D3": {Mentions: 2, LatestTS: "1700000100.000000"},
		"D1": {LatestTS: "1700000900.000000"},
	})
	return m
}

// names is the finder's rows in order, headings included.
func names(m Model) []string {
	known := map[string]bool{unreadsHeading: true, recentHeading: true, "ann": true, "bob": true, "cat": true, "dan-channel": true}
	var out []string
	for _, line := range strings.Split(ansi.Strip(m.View(80)), "\n") {
		for _, word := range strings.Fields(line) {
			if known[word] {
				out = append(out, word)
			}
		}
	}
	return out
}

// Slack's switcher opens on what is unread, then on where you were
// last. The finder listed everything by last visit, so an unread DM sat
// wherever its last visit put it.
func TestFinder_OpensOnUnreadsThenRecent(t *testing.T) {
	m := unreadFinder()
	m.Open()
	want := []string{unreadsHeading, "cat", "ann", recentHeading, "dan-channel", "bob"}
	if got := names(m); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("rows = %v\nwant   %v", got, want)
	}
	box := ansi.Strip(m.View(80))
	for _, line := range strings.Split(box, "\n") {
		if strings.Contains(line, "cat") && !strings.Contains(line, " 2 ") {
			t.Errorf("cat's row has no count: %q", line)
		}
		if strings.Contains(line, "ann") && strings.ContainsAny(line, "0123456789") {
			t.Errorf("ann is unread with no mentions and must show no count: %q", line)
		}
	}

	w, h := m.BoxSize(80, 24)
	rendered := m.renderBox(80)
	if w != lipgloss.Width(rendered) || h != lipgloss.Height(rendered) {
		t.Errorf("BoxSize = %dx%d, rendered %dx%d", w, h, lipgloss.Width(rendered), lipgloss.Height(rendered))
	}

	// Rows on screen: heading, cat, ann, heading, dan-channel, bob.
	if m.ClickRow(80, 24, listTopOffset()) || m.ClickRow(80, 24, listTopOffset()+3) {
		t.Error("a click on a heading selected a row")
	}
	if !m.ClickRow(80, 24, listTopOffset()+2) || m.items[m.filtered[m.selected]].Name != "ann" {
		t.Errorf("click on the third row selected %q, want ann", m.items[m.filtered[m.selected]].Name)
	}
	if !m.ClickRow(80, 24, listTopOffset()+4) || m.items[m.filtered[m.selected]].Name != "dan-channel" {
		t.Errorf("click on the fifth row selected %q, want dan-channel", m.items[m.filtered[m.selected]].Name)
	}
}

// A query ranks by match as before, with no headings, and still shows a
// count. Forwarding picks a destination by name and shows neither.
func TestFinder_HeadingsOnlyOnAnEmptyQuery(t *testing.T) {
	m := unreadFinder()
	m.Open()
	m.HandleKey("a")
	box := ansi.Strip(m.View(80))
	if strings.Contains(box, unreadsHeading) || strings.Contains(box, recentHeading) {
		t.Errorf("a query still shows the headings:\n%s", box)
	}
	m.HandleKey("backspace")
	m.HandleKey("c")
	if box := ansi.Strip(m.View(80)); !strings.Contains(box, " 2 ") {
		t.Errorf("a query's result for cat lost its count:\n%s", box)
	}

	f := unreadFinder()
	f.OpenForForwarding()
	if got, want := names(f), []string{"dan-channel", "cat", "bob", "ann"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("forwarding rows = %v, want %v by last visit", got, want)
	}
	if box := ansi.Strip(f.View(80)); strings.Contains(box, " 2 ") {
		t.Errorf("forwarding shows an unread count:\n%s", box)
	}

	none := unreadFinder()
	none.SetUnreads(nil)
	none.Open()
	if got := names(none); got[0] != "dan-channel" {
		t.Errorf("with nothing unread rows = %v, want no headings and the last visit first", got)
	}
}
