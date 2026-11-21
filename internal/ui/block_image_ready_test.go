package ui

import (
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// A Block Kit image (a /giphy post) or a link preview's picture is
// drawn as a "loading…" placeholder until its download lands. The
// message has to be drawn again then, or the placeholder stays until
// something else rebuilds the pane.
func TestBlockImageReadyMsg_RedrawsItsMessage(t *testing.T) {
	a, w1, _ := twoWindowApp(t)
	before := a.winModels[w1].Version()
	_, _ = a.Update(blockkit.BlockImageReadyMsg{Channel: "general", TS: "1.0", URL: "https://media.example.com/a.gif"})
	if a.winModels[w1].Version() == before {
		t.Fatal("a block image landing did not redraw the pane showing its channel")
	}

	before = a.winModels[w1].Version()
	_, _ = a.Update(blockkit.BlockImageReadyMsg{Channel: "elsewhere", TS: "1.0", URL: "https://media.example.com/a.gif"})
	if a.winModels[w1].Version() != before {
		t.Error("a block image for another channel redrew this pane")
	}
}

func TestBlockImageReadyMsg_RedrawsTheThreadShowingIt(t *testing.T) {
	app := NewApp()
	parent := messages.MessageItem{TS: "1.0", UserID: "U1", UserName: "alice"}
	reply := messages.MessageItem{TS: "1.001", UserID: "U2", UserName: "bob"}
	app.threadPanel.SetThread(parent, []messages.MessageItem{reply}, "C1", "1.0")
	_ = app.threadPanel.View(20, 60)

	for _, ts := range []string{"1.001", "1.0"} {
		before := app.threadPanel.Version()
		_, _ = app.Update(blockkit.BlockImageReadyMsg{Channel: "C1", TS: ts, URL: "https://media.example.com/a.gif"})
		if app.threadPanel.Version() == before {
			t.Errorf("a block image on %s did not redraw the open thread", ts)
		}
		_ = app.threadPanel.View(20, 60)
	}
	before := app.threadPanel.Version()
	_, _ = app.Update(blockkit.BlockImageReadyMsg{Channel: "C9", TS: "999.999", URL: "https://media.example.com/a.gif"})
	if app.threadPanel.Version() != before {
		t.Error("a block image on a message outside the thread redrew it")
	}
}
