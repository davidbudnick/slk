// internal/ui/messages/prepend_test.go
//
// PrependMessages boundary guard: an older-history backfill that
// raced a channel re-select (the re-select replaced the buffer while
// the backfill was in flight) can deliver items the buffer already
// contains. Items not strictly older than the buffer head are
// overlap/duplicates and must be dropped.
package messages

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func newPrependTestModel() *Model {
	m := New([]MessageItem{
		{TS: "1717171000.000100", UserName: "alice", UserID: "U1", Text: "head", Timestamp: "1:00 PM"},
		{TS: "1717171000.000200", UserName: "bob", UserID: "U2", Text: "tail", Timestamp: "1:01 PM"},
	}, "general")
	return m
}

func TestPrependMessages_DropsItemsNotOlderThanHead(t *testing.T) {
	m := newPrependTestModel()
	m.PrependMessages([]MessageItem{
		{TS: "1717170999.000050", UserName: "zoe", UserID: "U9", Text: "older", Timestamp: "12:59 PM"},   // strictly older: kept
		{TS: "1717171000.000100", UserName: "zoe", UserID: "U9", Text: "dup-head", Timestamp: "1:00 PM"}, // == head: dropped
		{TS: "1717171000.000150", UserName: "zoe", UserID: "U9", Text: "overlap", Timestamp: "1:00 PM"},  // inside buffer: dropped
	})
	msgs := m.Messages()
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages (1 kept + 2 baseline), got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].TS != "1717170999.000050" {
		t.Fatalf("kept item must be at the head, got TS %q", msgs[0].TS)
	}
	if msgs[1].TS != "1717171000.000100" || msgs[2].TS != "1717171000.000200" {
		t.Fatalf("baseline must be intact, got %q %q", msgs[1].TS, msgs[2].TS)
	}
}

func TestPrependMessages_AllOverlapIsNoOp(t *testing.T) {
	m := newPrependTestModel()
	m.PrependMessages([]MessageItem{
		{TS: "1717171000.000100", UserName: "zoe", UserID: "U9", Text: "dup-head", Timestamp: "1:00 PM"},
	})
	if got := len(m.Messages()); got != 2 {
		t.Fatalf("fully-overlapping prepend must be a no-op, got %d messages", got)
	}
}

func TestPrependMessages_EmptyBufferPrependsAll(t *testing.T) {
	m := New(nil, "general")
	m.PrependMessages([]MessageItem{
		{TS: "1717171000.000100", UserName: "alice", UserID: "U1", Text: "first", Timestamp: "1:00 PM"},
	})
	if got := len(m.Messages()); got != 1 {
		t.Fatalf("prepend into empty buffer must keep all items, got %d", got)
	}
}

// msgsFrom makes n one-line messages, each by its own author, the first at ts base.
func msgsFrom(base, n int) []MessageItem {
	out := make([]MessageItem, n)
	for i := range out {
		ts := fmt.Sprintf("17171%05d.000100", base+i)
		out[i] = MessageItem{TS: ts, UserName: fmt.Sprintf("u%d", base+i), UserID: fmt.Sprintf("U%d", base+i), Text: fmt.Sprintf("message %d", base+i), Timestamp: "1:00 PM"}
	}
	return out
}

// Older history landing while the user scrolls up keeps the message at
// the window's top where it was, on the first frame drawn after it.
func TestPrependMessages_KeepsTheWindowOnTheSameMessage(t *testing.T) {
	m := New(msgsFrom(100, 20), "general")
	_ = m.View(12, 80)
	for range 40 {
		m.ScrollUp(1)
		_ = m.View(12, 80)
	}
	before := withoutScrollbar(m.View(12, 80))
	if !strings.Contains(before, "message 103") {
		t.Fatalf("not near the top before the prepend (offset %d):\n%s", m.YOffset(), before)
	}

	m.PrependMessages(msgsFrom(50, 20))
	after := withoutScrollbar(m.View(12, 80))
	if after != before {
		t.Errorf("the first frame after the prepend moved:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if again := withoutScrollbar(m.View(12, 80)); again != before {
		t.Errorf("the second frame after the prepend moved:\n%s", again)
	}
}

// withoutScrollbar is a view's text without its last column, the
// scrollbar, whose thumb moves when history is added above.
func withoutScrollbar(view string) string {
	lines := strings.Split(ansi.Strip(view), "\n")
	for i, l := range lines {
		lines[i] = ansi.Cut(l, 0, max(0, ansi.StringWidth(l)-1))
	}
	return strings.Join(lines, "\n")
}
