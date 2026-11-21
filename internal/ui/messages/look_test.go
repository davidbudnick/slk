package messages

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/bubbles/entrylist"
	"github.com/gammons/slk/internal/ui/imgrender"
)

// withLook installs l for one test.
func withLook(t *testing.T, l Look) {
	t.Helper()
	SetLook(l)
	t.Cleanup(func() { SetLook(Look{}) })
}

// A bare link loses its scheme and trailing slash and gains the glyph, only in Slack's look.
func TestBareLinkLabel(t *testing.T) {
	long := "https://docs.example.com/" + strings.Repeat("a", 80)
	for _, tc := range []struct {
		name string
		look Look
		url  string
		want string
	}{
		{"slk's own look", Look{LinkGlyph: "@"}, "https://a.example/x/", "https://a.example/x/"},
		{"Slack's look", Look{Slack: true, LinkGlyph: "@"}, "https://a.example/x/", "@ a.example/x"},
		{"http", Look{Slack: true, LinkGlyph: "@"}, "http://a.example", "@ a.example"},
		{"no glyph", Look{Slack: true}, "https://a.example/x", "a.example/x"},
		{"an address", Look{Slack: true, LinkGlyph: "@"}, "mailto:sam@example.com", "sam@example.com"},
		{"cut at Slack's length", Look{Slack: true}, long, "docs.example.com/" + strings.Repeat("a", bareLinkLimit-len("docs.example.com/")) + "…"},
	} {
		SetLook(tc.look)
		if got := bareLinkLabel(tc.url); got != tc.want {
			t.Errorf("%s: bareLinkLabel(%q) = %q, want %q", tc.name, tc.url, got, tc.want)
		}
	}
	SetLook(Look{})
}

// A label that only repeats its address is drawn as the bare link; any other label stays.
func TestSlackLook_LabelRepeatingTheAddress(t *testing.T) {
	withLook(t, Look{Slack: true, LinkGlyph: "@"})
	for src, want := range map[string]string{
		"<http://a.example/x|a.example/x>":                         "@ a.example/x",
		"<https://a.example/docs/page#part|a.example/docs/page#…>": "@ a.example/docs/page#part",
		"<https://a.example/docs/page|a.example/do...>":            "@ a.example/docs/page",
		"<https://a.example/x|the docs>":                           "the docs",
		"<https://a.example/x|…>":                                  "…",
		"see <https://a.example/x/>":                               "see @ a.example/x",
	} {
		if got := ansi.Strip(RenderSlackMarkdown(src, nil, nil)); got != want {
			t.Errorf("%q drew %q, want %q", src, got, want)
		}
	}
}

// Links are underlined in slk's look, and in Slack's only when Slack's own setting says so.
func TestLinkStyleUnderline(t *testing.T) {
	for _, tc := range []struct {
		look Look
		want bool
	}{
		{Look{}, true},
		{Look{Slack: true}, false},
		{Look{Slack: true, UnderlineLinks: true}, true},
	} {
		SetLook(tc.look)
		if got := linkStyle().GetUnderline(); got != tc.want {
			t.Errorf("%+v: underline = %v, want %v", tc.look, got, tc.want)
		}
	}
	SetLook(Look{})
}

// Slack's times are upright; slk's own are italic.
func TestTimestampStyle(t *testing.T) {
	if !TimestampStyle().GetItalic() {
		t.Error("default look: the time is not italic")
	}
	withLook(t, Look{Slack: true})
	if TimestampStyle().GetItalic() {
		t.Error("Slack's look: the time is italic")
	}
}

// Slack's day label sits on a rule across the pane; the unread line ends in "New".
func TestSlackLook_Separators(t *testing.T) {
	if got := strings.TrimSpace(ansi.Strip(DaySeparator("Today", 40))); got != "── Today ──" {
		t.Errorf("default day separator = %q", got)
	}
	if got := strings.TrimSpace(ansi.Strip(NewSeparator(40))); got != "── new ──" {
		t.Errorf("default unread line = %q", got)
	}
	withLook(t, Look{Slack: true})
	day := ansi.Strip(DaySeparator("Today", 40))
	if lipgloss.Width(day) != 40 || !strings.HasPrefix(day, "────") || !strings.HasSuffix(day, "────") || !strings.Contains(day, " Today ") {
		t.Errorf("Slack day separator = %q, want the label on a 40-column rule", day)
	}
	unread := ansi.Strip(NewSeparator(40))
	if lipgloss.Width(unread) != 40 || !strings.HasPrefix(unread, "────") || !strings.HasSuffix(unread, " New ") {
		t.Errorf("Slack unread line = %q, want a 40-column rule ending in New", unread)
	}
}

// In Slack's look reactions end with an add pill that has its own hit.
func TestSlackLook_AddReactionPill(t *testing.T) {
	msg := MessageItem{
		TS: "1700000000.000000", UserID: "U1", UserName: "alice", Text: "hi", Timestamp: "9:00 AM",
		Reactions: []ReactionItem{{Emoji: "tada", Count: 2}},
	}
	addHits := func() int {
		m := New([]MessageItem{msg}, "general")
		n := 0
		for _, h := range RenderMessage(msg, 80, m.renderContext(), RenderOptions{}).Hits {
			if h.Kind == entrylist.HitReaction && h.Key == AddReactionKey {
				n++
			}
		}
		return n
	}
	if got := addHits(); got != 0 {
		t.Errorf("default look: %d add pills, want none", got)
	}
	withLook(t, Look{Slack: true})
	if got := addHits(); got != 1 {
		t.Errorf("Slack's look: %d add pills, want 1", got)
	}
	msg.Reactions = nil
	if got := addHits(); got != 0 {
		t.Errorf("a message with no reactions drew %d add pills", got)
	}
}

// Slack draws a message's reactions over its reply row, and the pills'
// hits follow them up a row.
func TestSlackLook_ReactionsOverTheReplyRow(t *testing.T) {
	msg := MessageItem{
		TS: "1700000000.000000", UserID: "U1", UserName: "alice", Text: "hi", Timestamp: "9:00 AM",
		ReplyCount: 1, Reactions: []ReactionItem{{Emoji: "tada", Count: 2}},
	}
	layout := func() (pillRow, replyRow, hitRow int) {
		m := New([]MessageItem{msg}, "general")
		r := RenderMessage(msg, 80, m.renderContext(), RenderOptions{ReplyAffordance: true})
		pillRow, replyRow, hitRow = -1, -1, -1
		for i, line := range strings.Split(ansi.Strip(r.Content), "\n") {
			switch {
			case strings.Contains(line, "1 reply"):
				replyRow = i
			case strings.Contains(line, "2"):
				pillRow = i
			}
		}
		for _, h := range r.Hits {
			if h.Kind == entrylist.HitReaction && h.Key == "tada" {
				hitRow = h.RowStart
			}
		}
		return pillRow, replyRow, hitRow
	}
	if pill, reply, hit := layout(); pill != reply+1 || hit != pill {
		t.Errorf("default look: pills on row %d (hit %d), reply row %d; want the pills under it", pill, hit, reply)
	}
	withLook(t, Look{Slack: true})
	if pill, reply, hit := layout(); reply != pill+1 || hit != pill {
		t.Errorf("Slack's look: pills on row %d (hit %d), reply row %d; want the pills over it", pill, hit, reply)
	}
}

// Inline images are named, a run of them counted, and only in Slack's look.
func TestFileLabel(t *testing.T) {
	one := []imgrender.Block{{Name: "Screenshot 2026-03-15 at 9.00.00 AM.png"}}
	if got := fileLabel(one, 60); got != "" {
		t.Errorf("default look drew a label: %q", got)
	}
	withLook(t, Look{Slack: true})
	if got := ansi.Strip(fileLabel(one, 60)); got != one[0].Name {
		t.Errorf("label = %q, want the file's name", got)
	}
	if got := ansi.Strip(fileLabel(append(one, imgrender.Block{Name: "b.png"}), 60)); got != "2 files" {
		t.Errorf("label of a run = %q, want %q", got, "2 files")
	}
	if got := ansi.Strip(fileLabel(one, 12)); lipgloss.Width(got) != 12 || !strings.HasSuffix(got, "…") {
		t.Errorf("label in 12 columns = %q", got)
	}
	if got := fileLabel(nil, 60); got != "" {
		t.Errorf("label with no inline image = %q", got)
	}
}
