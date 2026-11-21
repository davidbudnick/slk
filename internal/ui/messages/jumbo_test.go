package messages

import (
	goimage "image"
	"io"
	"testing"

	emojiutil "github.com/gammons/slk/internal/emoji"
	imgpkg "github.com/gammons/slk/internal/image"
)

// jumboOpts is render options whose fetcher has :thumbsup: ready at jumbo size.
func jumboOpts(t *testing.T, flushes *[]func(io.Writer) error) RenderSlackMarkdownOpts {
	t.Helper()
	emojiutil.SetImageMode(true, 2)
	t.Cleanup(func() { emojiutil.SetImageMode(false, 2) })
	ff := newFakePlaceFetcher()
	ff.setPrerendered(emojiutil.EmojiCacheKey(emojiutil.CDNBaseURL+"1f44d.png"), goimage.Pt(jumboCells, jumboRows), imgpkg.Render{
		Cells:   goimage.Pt(jumboCells, jumboRows),
		Lines:   []string{"AAAA", "BBBB"},
		OnFlush: func(io.Writer) error { return nil },
	})
	return RenderSlackMarkdownOpts{PlaceCtx: emojiutil.PlaceContext{Fetcher: ff}, EmojiCells: 2, EmojiFlushes: flushes}
}

// A message of nothing but emoji is drawn two rows tall, as Slack draws it.
func TestJumboEmoji(t *testing.T) {
	var flushes []func(io.Writer) error
	opts := jumboOpts(t, &flushes)
	withLook(t, Look{Slack: true})

	for _, tc := range []struct {
		name  string
		text  string
		width int
		want  string
	}{
		{"one", ":thumbsup:", 40, "AAAA\nBBBB"},
		{"two share the rows", " :thumbsup: :thumbsup:\n", 40, "AAAA AAAA\nBBBB BBBB"},
		{"a narrow pane wraps whole emoji", ":thumbsup::thumbsup::thumbsup:", 9, "AAAA AAAA\nBBBB BBBB\nAAAA\nBBBB"},
	} {
		flushes = nil
		got, ok := jumboEmoji(tc.text, opts, tc.width)
		if !ok || got != tc.want {
			t.Errorf("%s: jumboEmoji = %q, %v; want %q", tc.name, got, ok, tc.want)
		}
		if len(flushes) == 0 {
			t.Errorf("%s: no upload collected for the placed emoji", tc.name)
		}
	}
}

// Text beside the emoji, too many of them, or either setting off keeps the ordinary size.
func TestJumboEmoji_NotJumbo(t *testing.T) {
	opts := jumboOpts(t, nil)
	many := ""
	for range jumboLimit + 1 {
		many += ":thumbsup:"
	}
	for _, tc := range []struct {
		name string
		look Look
		text string
	}{
		{"words too", Look{Slack: true}, "nice :thumbsup:"},
		{"no emoji", Look{Slack: true}, "nice"},
		{"past Slack's limit", Look{Slack: true}, many},
		{"slk's own look", Look{}, ":thumbsup:"},
		{"Slack's jumbo emoji switched off", Look{Slack: true, PlainEmoji: true}, ":thumbsup:"},
	} {
		SetLook(tc.look)
		if got, ok := jumboEmoji(tc.text, opts, 40); ok {
			t.Errorf("%s: drawn jumbo as %q", tc.name, got)
		}
	}
	SetLook(Look{})
}

// The message pane gives a jumbo message its two rows.
func TestRenderMessage_JumboBody(t *testing.T) {
	var flushes []func(io.Writer) error
	opts := jumboOpts(t, &flushes)
	withLook(t, Look{Slack: true})
	msg := MessageItem{TS: "1700000000.000000", UserID: "U1", UserName: "alice", Text: ":thumbsup:", Timestamp: "9:00 AM"}
	m := New([]MessageItem{msg}, "general")
	m.SetEmojiContext(EmojiContext{PlaceCtx: opts.PlaceCtx, Cells: 2})
	r := RenderMessage(msg, 80, m.renderContext(), RenderOptions{BodyOnly: true})
	if got := stripKeepingLinks(r.Content); got != "AAAA\nBBBB" {
		t.Errorf("body = %q, want the emoji's two rows", got)
	}
}
