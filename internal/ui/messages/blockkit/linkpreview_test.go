package blockkit

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	imgpkg "github.com/gammons/slk/internal/image"
)

// samplePreview is the attachment Slack makes of a pasted store link.
func samplePreview() LegacyAttachment {
	return LegacyAttachment{
		Title:       "Cardgame on Store",
		TitleLink:   "https://store.example.com/app/1/Cardgame/",
		Text:        strings.Repeat("A deckbuilder where you play illegal poker hands. ", 8),
		OriginalURL: "https://store.example.com/app/1/Cardgame/",
		ImageURL:    "https://cdn.example.com/header.jpg",
		ImageWidth:  616,
		ImageHeight: 353,
	}
}

// previewCtx is a Slack-look context that wraps text, with images drawn by proto.
func previewCtx(proto imgpkg.Protocol) Context {
	ctx := imageCtx(proto)
	ctx.Slack = true
	ctx.WrapText = func(s string, w int) string { return ansi.Wordwrap(s, w, "") }
	return ctx
}

// Only the plain preview of a pasted link is a card; an app's attachment keeps its stripe.
func TestIsLinkPreview(t *testing.T) {
	edit := func(f func(*LegacyAttachment)) LegacyAttachment {
		a := samplePreview()
		f(&a)
		return a
	}
	for _, tc := range []struct {
		name string
		att  LegacyAttachment
		want bool
	}{
		{"a pasted link", samplePreview(), true},
		{"from_url only", edit(func(a *LegacyAttachment) { a.OriginalURL, a.FromURL = "", "https://x.example" }), true},
		{"no link", edit(func(a *LegacyAttachment) { a.OriginalURL = "" }), false},
		{"no title", edit(func(a *LegacyAttachment) { a.Title = "" }), false},
		{"an app's colour", edit(func(a *LegacyAttachment) { a.Color = "24292f" }), false},
		{"a shared message", edit(func(a *LegacyAttachment) { a.AuthorName = "Sam" }), false},
		{"a footer", edit(func(a *LegacyAttachment) { a.Footer = "acme/menus" }), false},
		{"fields", edit(func(a *LegacyAttachment) { a.Fields = []LegacyField{{Title: "State"}} }), false},
		{"nested blocks", edit(func(a *LegacyAttachment) { a.Blocks = []Block{DividerBlock{}} }), false},
	} {
		if got := isLinkPreview(tc.att); got != tc.want {
			t.Errorf("%s: isLinkPreview = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The card is a frame of one width holding the title, three rows of text and the site.
func TestLinkPreviewCardWithoutAnImage(t *testing.T) {
	ctx := previewCtx(imgpkg.ProtoOff)
	res := RenderLegacy([]LegacyAttachment{samplePreview()}, ctx, 60)
	if len(res.Lines) != 7 {
		t.Fatalf("card is %d rows, want 7 (frame, title, 3 of text, site, frame):\n%s", len(res.Lines), strings.Join(res.Lines, "\n"))
	}
	for i, l := range res.Lines {
		if w := lipgloss.Width(l); w != 60 {
			t.Errorf("row %d is %d wide, want 60: %q", i, w, ansi.Strip(l))
		}
	}
	plain := strings.Split(ansi.Strip(strings.Join(res.Lines, "\n")), "\n")
	if !strings.HasPrefix(plain[0], "╭") || !strings.HasPrefix(plain[6], "╰") {
		t.Errorf("no frame around the card: %q … %q", plain[0], plain[6])
	}
	if !strings.Contains(plain[1], "Cardgame on Store") {
		t.Errorf("row 1 = %q, want the title", plain[1])
	}
	if !strings.Contains(plain[4], "…") {
		t.Errorf("row 4 = %q, want the cut-off text to end in an ellipsis", plain[4])
	}
	if !strings.Contains(plain[5], "store.example.com") {
		t.Errorf("row 5 = %q, want the site", plain[5])
	}
	if strings.Contains(strings.Join(plain, "\n"), "[image]") {
		t.Error("the card names its image as a link; Slack shows none when it cannot draw it")
	}
}

// The image sits at the card's right, and its hit and sixel placement name that column.
func TestLinkPreviewCardImageAtTheRight(t *testing.T) {
	const width, textW, thumbCols = 60, 40, 14
	thumbCol := 2 + textW + 2
	for _, proto := range []imgpkg.Protocol{imgpkg.ProtoHalfBlock, imgpkg.ProtoSixel} {
		res := RenderLegacy([]LegacyAttachment{samplePreview()}, previewCtx(proto), width)
		for i, l := range res.Lines {
			if w := lipgloss.Width(l); w != width {
				t.Errorf("proto %v: row %d is %d wide, want %d", proto, i, w, width)
			}
		}
		if len(res.Hits) != 1 {
			t.Fatalf("proto %v: %d hits, want the image's", proto, len(res.Hits))
		}
		want := HitRect{RowStart: 1, RowEnd: 1 + previewThumbRows, ColStart: thumbCol, ColEnd: thumbCol + thumbCols}
		if res.Hits[0] != want {
			t.Errorf("proto %v: hit = %+v, want %+v", proto, res.Hits[0], want)
		}
		if proto != imgpkg.ProtoSixel {
			continue
		}
		entry, ok := res.SixelRows[1]
		if !ok || entry.Col != thumbCol {
			t.Errorf("sixel placement = %+v (found %v), want row 1 at column %d", entry, ok, thumbCol)
		}
	}
}

// Without Slack's look a preview keeps slk's stripe.
func TestLinkPreviewKeepsTheStripeByDefault(t *testing.T) {
	ctx := previewCtx(imgpkg.ProtoOff)
	ctx.Slack = false
	res := RenderLegacy([]LegacyAttachment{samplePreview()}, ctx, 60)
	if plain := ansi.Strip(strings.Join(res.Lines, "\n")); !strings.Contains(plain, stripeGlyph) || strings.Contains(plain, "╭") {
		t.Errorf("default look drew:\n%s", plain)
	}
}

// A footer is dated the way Slack dates one.
func TestFooterTime(t *testing.T) {
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		when time.Time
		want string
	}{
		{time.Date(2026, 3, 15, 9, 5, 0, 0, time.UTC), "Today at 9:05 AM"},
		{time.Date(2026, 3, 14, 12, 45, 0, 0, time.UTC), "Yesterday at 12:45 PM"},
		{time.Date(2026, 3, 1, 16, 5, 0, 0, time.UTC), "Mar 1st at 4:05 PM"},
		{time.Date(2026, 2, 22, 16, 5, 0, 0, time.UTC), "Feb 22nd at 4:05 PM"},
		{time.Date(2026, 1, 13, 16, 5, 0, 0, time.UTC), "Jan 13th at 4:05 PM"},
		{time.Date(2025, 11, 3, 8, 0, 0, 0, time.UTC), "Nov 3rd, 2025 at 8:00 AM"},
	} {
		if got := footerTime(tc.when.Unix(), at); got != tc.want {
			t.Errorf("footerTime(%s) = %q, want %q", tc.when.Format(time.DateTime), got, tc.want)
		}
	}
}

// A footer's link is drawn as its label and stays a hyperlink.
func TestLegacyFooterLink(t *testing.T) {
	att := LegacyAttachment{Color: "24292f", Title: "#24", Footer: "<https://git.example.com/acme/menus|acme/menus>"}
	lines := RenderLegacy([]LegacyAttachment{att}, previewCtx(imgpkg.ProtoOff), 60).Lines
	footer := lines[len(lines)-1]
	if got := ansi.Strip(footer); !strings.HasSuffix(got, "acme/menus") || strings.Contains(got, "<") {
		t.Errorf("footer drew %q, want the label alone", got)
	}
	if !strings.Contains(footer, "\x1b]8;;https://git.example.com/acme/menus\x1b\\") {
		t.Errorf("footer lost its hyperlink: %q", footer)
	}
	if got := footerLinks("plain | <https://a.example/x>"); ansi.Strip(got) != "plain | https://a.example/x" {
		t.Errorf("unlabelled footer link drew %q", ansi.Strip(got))
	}
}

// In Slack's look an app attachment's footer carries Slack's date.
func TestLegacyFooterDateInSlackLook(t *testing.T) {
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return at }
	t.Cleanup(func() { now = time.Now })
	att := LegacyAttachment{Color: "24292f", Title: "#24", Footer: "acme/menus", TS: at.Add(-24 * time.Hour).Unix()}

	ctx := previewCtx(imgpkg.ProtoOff)
	got := ansi.Strip(strings.Join(RenderLegacy([]LegacyAttachment{att}, ctx, 60).Lines, "\n"))
	if want := "acme/menus | Yesterday at 12:00 PM"; !strings.Contains(got, want) {
		t.Errorf("Slack look footer missing %q:\n%s", want, got)
	}
	ctx.Slack = false
	got = ansi.Strip(strings.Join(RenderLegacy([]LegacyAttachment{att}, ctx, 60).Lines, "\n"))
	if want := "acme/menus · 2026-03-14 12:00 PM"; !strings.Contains(got, want) {
		t.Errorf("default footer missing %q:\n%s", want, got)
	}
}
