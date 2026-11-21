package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// foldApp is an app in Slack's look whose one message carries an
// attachment with more text than Slack shows at once.
func foldApp(t *testing.T) *App {
	t.Helper()
	const ts = "1700000001.000000"
	a := NewApp()
	t.Cleanup(func() {
		messages.SetLook(messages.Look{})
		if messages.Unfolded(ts) {
			messages.ToggleFold(ts)
		}
	})
	a.SetMessageSlackStyle(true)
	a.width = 120
	a.height = 40
	a.activeChannelID = "C1"
	a.focusedPanel = PanelMessages
	a.messagepane.SetMessages([]messages.MessageItem{{
		TS: ts, UserID: "U1", UserName: "alice", Text: "review please", Timestamp: "10:30 AM",
		LegacyAttachments: []blockkit.LegacyAttachment{{
			Color: "36a64f",
			Title: "#12 Stop logging the token",
			Text:  "one\ntwo\nthree\nfour\nfive\nthe sixth line",
		}},
	}})
	_ = a.View()
	return a
}

// showMoreCell returns a screen cell of the "Show more" or "Show less" row.
func showMoreCell(t *testing.T, a *App) (x, y int) {
	t.Helper()
	chrome := a.messagepane.ChromeHeight()
	for paneY := chrome; paneY < a.height-2; paneY++ {
		for paneX := 0; paneX < a.layout.msgEnd-a.layout.sidebarEnd-2; paneX++ {
			if _, key, ok := a.messagepane.HitTestReaction(paneY-chrome, paneX); ok && key == messages.ShowMoreKey {
				return a.layout.sidebarEnd + 1 + paneX, paneY + 1
			}
		}
	}
	t.Fatal("no Show more row in the messages pane")
	return 0, 0
}

// A click on "Show more" draws the attachment's whole text, a click on "Show less" folds it again, and e does both.
func TestApp_ShowMoreUnfoldsAnAttachment(t *testing.T) {
	a := foldApp(t)
	shows := func(text string) bool { return strings.Contains(ansi.Strip(a.View().Content), text) }
	if !shows("Show more") || shows("the sixth line") {
		t.Fatal("the long attachment is not folded behind Show more")
	}

	x, y := showMoreCell(t, a)
	a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if !shows("the sixth line") || !shows("Show less") {
		t.Fatal("a click on Show more did not unfold the attachment")
	}

	x, y = showMoreCell(t, a)
	a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if shows("the sixth line") {
		t.Fatal("a click on Show less did not fold the attachment")
	}

	_ = press(a, 'e')
	if !shows("the sixth line") {
		t.Error("e did not unfold the selected message's attachment")
	}
	_ = press(a, 'e')
	if shows("the sixth line") {
		t.Error("a second e did not fold it again")
	}
}

// e on a message with nothing folded says so and unfolds nothing.
func TestApp_ShowMoreKeyWithNothingFolded(t *testing.T) {
	a := foldApp(t)
	a.messagepane.SetMessages([]messages.MessageItem{{TS: "1700000002.000000", UserID: "U1", UserName: "alice", Text: "short", Timestamp: "10:31 AM"}})
	_ = a.View()
	_ = press(a, 'e')
	if !strings.Contains(statusbarText(a), "Nothing more to show") {
		t.Errorf("status bar = %q, want the toast", statusbarText(a))
	}
	if messages.Unfolded("1700000002.000000") {
		t.Error("e unfolded a message with nothing folded")
	}
}
