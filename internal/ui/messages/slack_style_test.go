package messages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// groupedMessages is alice writing three times inside five minutes, bob
// answering, and alice again twice, the second a long time after.
func groupedMessages() []MessageItem {
	return []MessageItem{
		{TS: "1700000000.000100", UserID: "U1", UserName: "alice", Text: "first", Timestamp: "10:00 AM"},
		{TS: "1700000060.000100", UserID: "U1", UserName: "alice", Text: "second", Timestamp: "10:01 AM"},
		{TS: "1700000120.000100", UserID: "U1", UserName: "alice", Text: "third", Timestamp: "10:02 AM"},
		{TS: "1700000180.000100", UserID: "U2", UserName: "bob", Text: "fourth", Timestamp: "10:03 AM"},
		{TS: "1700000240.000100", UserID: "U1", UserName: "alice", Text: "fifth", Timestamp: "10:04 AM"},
		{TS: "1700001200.000100", UserID: "U1", UserName: "alice", Text: "sixth", Timestamp: "10:20 AM"},
	}
}

// Slack shows an author's name once over the messages they send within
// a few minutes. Every message drawing its own header took a row each
// and repeated the name down the pane.
func TestSlackStyle_GroupsAnAuthorsMessagesUnderOneHeader(t *testing.T) {
	m := New(groupedMessages(), "general")
	plain := func() string { return ansi.Strip(m.View(40, 60)) }

	if got := strings.Count(plain(), "alice"); got != 5 {
		t.Fatalf("default: alice named %d times, want once per message (5)", got)
	}
	m.SetSlackStyle(true)
	out := plain()
	if got := strings.Count(out, "alice"); got != 3 {
		t.Errorf("alice named %d times, want 3: her first message, the one after bob, and the one twenty minutes on\n%s", got, out)
	}
	if got := strings.Count(out, "bob"); got != 1 {
		t.Errorf("bob named %d times, want 1", got)
	}
	for _, body := range []string{"first", "second", "third", "fourth", "fifth", "sixth"} {
		if !strings.Contains(out, body) {
			t.Errorf("message %q is missing", body)
		}
	}
	column := func(body string) int {
		for _, line := range strings.Split(out, "\n") {
			if i := strings.Index(line, body); i >= 0 {
				return ansi.StringWidth(line[:i])
			}
		}
		return -1
	}
	if column("second") != column("first") {
		t.Errorf("a grouped message starts at cell %d, its group's first at %d; want one column", column("second"), column("first"))
	}
}

// What keeps a message its own header: an edit mark to show, nothing to
// say but an attachment, the unread line above it, or compact mode.
func TestSlackStyle_WhatBreaksAGroup(t *testing.T) {
	names := func(m *Model) int { return strings.Count(ansi.Strip(m.View(40, 60)), "alice") }

	msgs := groupedMessages()[:3]
	msgs[1].IsEdited = true
	m := New(msgs, "general")
	m.SetSlackStyle(true)
	if got := names(m); got != 2 {
		t.Errorf("with the second message edited alice is named %d times, want 2", got)
	}

	msgs = groupedMessages()[:3]
	msgs[1].Text = ""
	m = New(msgs, "general")
	m.SetSlackStyle(true)
	if got := names(m); got != 2 {
		t.Errorf("with the second message empty alice is named %d times, want 2", got)
	}

	m = New(groupedMessages()[:3], "general")
	m.SetSlackStyle(true)
	m.SetLastReadTS("1700000060.000100")
	if got := names(m); got != 2 {
		t.Errorf("with the unread line before the third message alice is named %d times, want 2", got)
	}

	m = New(groupedMessages()[:3], "general")
	m.SetSlackStyle(true)
	m.SetCompact(true)
	if got := names(m); got != 3 {
		t.Errorf("compact mode names alice %d times, want every message (3)", got)
	}
}

// A conversation shorter than the pane sits at its bottom, by the
// input, as in Slack.
func TestSlackStyle_ShortConversationSitsAtTheBottom(t *testing.T) {
	m := New(groupedMessages()[:1], "general")
	rowOf := func() (row, rows int) {
		lines := strings.Split(ansi.Strip(m.View(20, 60)), "\n")
		for i, line := range lines {
			if strings.Contains(line, "first") {
				return i, len(lines)
			}
		}
		return -1, len(lines)
	}
	top, _ := rowOf()
	m.SetSlackStyle(true)
	bottom, rows := rowOf()
	if top < 0 || bottom <= top || bottom != rows-1 {
		t.Errorf("message on row %d by default and %d of %d in Slack style; want it moved to the last row", top, bottom, rows)
	}
}
