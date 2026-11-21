package sidebar

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

// openRowSidebar has C1 open in the message pane and the cursor on C2.
func openRowSidebar() Model {
	m := New([]ChannelItem{
		{ID: "C1", Name: "general", Type: "channel", Section: "Team"},
		{ID: "C2", Name: "random", Type: "channel", Section: "Team"},
	})
	m.SetActiveChannelID("C1")
	m.SelectByID("C2")
	return m
}

// marked reports whether the named row carries the "▌" marker.
func marked(t *testing.T, m *Model, name string) bool {
	t.Helper()
	return strings.Contains(rowFor(t, m.View(12, 30), name), "▌")
}

// The pane shows one destination, so one row is marked as open: going
// from Activity to Threads used to leave both marked, and the channel
// behind them too.
func TestOpenRow_OnlyTheShownDestinationIsMarked(t *testing.T) {
	m := openRowSidebar()
	if !marked(t, &m, "general") || marked(t, &m, "Threads") || marked(t, &m, "Activity") {
		t.Fatal("with a channel open, only its row must be marked")
	}
	m.SetActivityActive(true)
	if !marked(t, &m, "Activity") || marked(t, &m, "Threads") || marked(t, &m, "general") {
		t.Error("with Activity up, only the Activity row must be marked")
	}
	m.SetThreadsActive(true)
	if !marked(t, &m, "Threads") || marked(t, &m, "Activity") || marked(t, &m, "general") {
		t.Error("with Threads up, only the Threads row must be marked")
	}
	m.SetThreadsActive(false)
	if !marked(t, &m, "general") || marked(t, &m, "Threads") || marked(t, &m, "Activity") {
		t.Error("back on the channel, only its row must be marked")
	}
}

// Slack style fills the open row, as Slack fills its open conversation,
// in place of the marker, and keeps it filled under the cursor.
func TestOpenRow_SlackStyleFillsIt(t *testing.T) {
	m := openRowSidebar()
	m.SetSlackStyle(true)
	tint := messages.SelectionTintBgANSI(true)

	open := rowFor(t, m.View(12, 30), "general")
	if !strings.Contains(open, tint) || strings.Contains(open, "▌") {
		t.Errorf("open row must be filled and unmarked: %q", open)
	}
	if other := rowFor(t, m.View(12, 30), "random"); strings.Contains(other, tint) {
		t.Errorf("cursor row away from the open one must not be filled: %q", other)
	}
	m.SelectByID("C1")
	under := rowFor(t, m.View(12, 30), "general")
	if !strings.Contains(under, tint) || !strings.Contains(under, "▌") {
		t.Errorf("open row under the cursor must stay filled and show the cursor: %q", under)
	}
	m.SetThreadsActive(true)
	if row := rowFor(t, m.View(12, 30), "Threads"); !strings.Contains(row, tint) {
		t.Errorf("Threads row must be filled while the Threads view is up: %q", row)
	}
	if row := rowFor(t, m.View(12, 30), "Activity"); strings.Contains(row, tint) {
		t.Errorf("Activity row filled while Threads is up: %q", row)
	}
	for _, name := range []string{"Threads", "general"} {
		if w := lipgloss.Width(rowFor(t, m.View(12, 30), name)); w != 28 {
			t.Errorf("filled %s row is %d cells wide, want 28 like every other row", name, w)
		}
	}
}
