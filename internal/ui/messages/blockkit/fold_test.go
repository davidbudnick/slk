package blockkit

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/styles"
)

// longText is a pull request's description as an app unfurls it: five
// lines Slack shows, then more.
const longText = "## Context\n\nThe callback carries a token.\n\n• Logs kept it.\n• Traces kept it.\n\nThis removes it from both.\n\n## Changes\n\n• Redact the address."

// Slack shows five lines, blank ones aside, or 700 characters, and folds the rest.
func TestFoldText(t *testing.T) {
	words := strings.Repeat("word ", 200)
	for _, tc := range []struct {
		name, text, want string
		folded           bool
	}{
		{"short", "one\ntwo", "one\ntwo", false},
		{"five lines", "1\n2\n\n3\n4\n5\n", "1\n2\n\n3\n4\n5", false},
		{"six lines", "1\n2\n\n3\n4\n5\n6", "1\n2\n\n3\n4\n5", true},
		{"a description", longText, longText[:strings.Index(longText, "\n\n## Changes")], true},
		{"one long paragraph", words, strings.TrimSpace(words[:foldChars]) + "…", true},
		{"a cut inside a link", strings.Repeat("a", foldChars-30) + " <https://a.example/x|a long label of a link> end", strings.Repeat("a", foldChars-30) + "…", true},
	} {
		got, folded := foldText(tc.text)
		if got != tc.want || folded != tc.folded {
			t.Errorf("%s: foldText = %q, %v; want %q, %v", tc.name, got, folded, tc.want, tc.folded)
		}
		if n := utf8.RuneCountInString(got); n > foldChars+1 {
			t.Errorf("%s: %d characters shown, want at most %d", tc.name, n, foldChars+1)
		}
	}
}

// unfurl is an app's unfurl of a pull request: a linked title, a long description, a field and a footer.
func unfurl() LegacyAttachment {
	return LegacyAttachment{
		Color:     "36a64f",
		Title:     "#12 Stop logging the token",
		TitleLink: "https://git.example/acme/bridge/pull/12",
		Text:      longText,
		Fields:    []LegacyField{{Title: "Comments", Value: "4", Short: true}},
		Footer:    "acme/bridge",
	}
}

// rowOf returns the index of the row holding text, or -1.
func rowOf(lines []string, text string) int {
	for i, l := range lines {
		if strings.Contains(ansi.Strip(l), text) {
			return i
		}
	}
	return -1
}

// In Slack's look long attachment text stops at "Show more", whose row is the fold's hit; unfolded it ends in "Show less".
func TestLegacyTextFoldsBehindShowMore(t *testing.T) {
	ctx := previewCtx(imgpkg.ProtoOff)
	res := RenderLegacy([]LegacyAttachment{unfurl()}, ctx, 60)
	more := rowOf(res.Lines, showMore)
	if more < 0 || rowOf(res.Lines, "## Changes") >= 0 {
		t.Fatalf("folded text drew:\n%s", ansi.Strip(strings.Join(res.Lines, "\n")))
	}
	if rowOf(res.Lines, "This removes it from both.") != more-1 {
		t.Errorf("%q is not right under the fifth line:\n%s", showMore, ansi.Strip(strings.Join(res.Lines, "\n")))
	}
	if rowOf(res.Lines, "Comments") < more || rowOf(res.Lines, "acme/bridge") < more {
		t.Errorf("the field and footer are not under %q:\n%s", showMore, ansi.Strip(strings.Join(res.Lines, "\n")))
	}
	want := HitRect{RowStart: more, RowEnd: more + 1, ColStart: stripeCol, ColEnd: stripeCol + len(showMore)}
	if len(res.Folds) != 1 || res.Folds[0] != want {
		t.Errorf("fold hits = %+v, want %+v", res.Folds, want)
	}

	ctx.Unfolded = true
	res = RenderLegacy([]LegacyAttachment{unfurl()}, ctx, 60)
	less := rowOf(res.Lines, showLess)
	if less < 0 || rowOf(res.Lines, "Redact the address.") != less-1 || rowOf(res.Lines, showMore) >= 0 {
		t.Errorf("unfolded text drew:\n%s", ansi.Strip(strings.Join(res.Lines, "\n")))
	}
	if len(res.Folds) != 1 || res.Folds[0].RowStart != less {
		t.Errorf("fold hits = %+v, want the %q row %d", res.Folds, showLess, less)
	}

	ctx.Slack, ctx.Unfolded = false, false
	res = RenderLegacy([]LegacyAttachment{unfurl()}, ctx, 60)
	if rowOf(res.Lines, showMore) >= 0 || rowOf(res.Lines, "Redact the address.") < 0 || len(res.Folds) != 0 {
		t.Errorf("slk's own look folded the text:\n%s", ansi.Strip(strings.Join(res.Lines, "\n")))
	}
}

// HasFold is true for an attachment whose text folds, and not for a link preview card, which cuts its own text.
func TestHasFold(t *testing.T) {
	short := unfurl()
	short.Text = "one line"
	card := samplePreview()
	card.Text = longText
	for _, tc := range []struct {
		name string
		atts []LegacyAttachment
		want bool
	}{
		{"long text", []LegacyAttachment{short, unfurl()}, true},
		{"short text", []LegacyAttachment{short}, false},
		{"a link preview", []LegacyAttachment{card}, false},
		{"none", nil, false},
	} {
		if got := HasFold(tc.atts); got != tc.want {
			t.Errorf("%s: HasFold = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// In Slack's look a linked title takes the link colour, and the service and author head the attachment.
func TestLegacyTitleAndBylineInSlackLook(t *testing.T) {
	a := unfurl()
	a.ServiceName, a.AuthorName = "Git", "Sam Doe"
	link := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary).Render("x")
	colour := link[:strings.Index(link, "x")]

	ctx := previewCtx(imgpkg.ProtoOff)
	res := RenderLegacy([]LegacyAttachment{a}, ctx, 60)
	if got := ansi.Strip(res.Lines[0]); !strings.HasSuffix(got, "Git | Sam Doe") {
		t.Errorf("first row = %q, want the service and author", got)
	}
	if title := res.Lines[rowOf(res.Lines, "#12 Stop logging")]; !strings.Contains(title, colour) {
		t.Errorf("title is not in the link colour: %q", title)
	}

	ctx.Slack = false
	res = RenderLegacy([]LegacyAttachment{a}, ctx, 60)
	if rowOf(res.Lines, "Sam Doe") >= 0 || strings.Contains(res.Lines[0], colour) {
		t.Errorf("slk's own look drew:\n%s", ansi.Strip(strings.Join(res.Lines, "\n")))
	}
}

// A preview that names its author is still a card, with the author beside the site.
func TestLinkPreviewNamesTheAuthor(t *testing.T) {
	a := samplePreview()
	a.Text, a.ServiceName, a.AuthorName = "", "Vids", "Sam Doe"
	res := RenderLegacy([]LegacyAttachment{a}, previewCtx(imgpkg.ProtoOff), 60)
	plain := ansi.Strip(strings.Join(res.Lines, "\n"))
	if !strings.HasPrefix(plain, "╭") || !strings.Contains(plain, "Vids | Sam Doe") {
		t.Errorf("card drew:\n%s", plain)
	}
}

// In Slack's look a section's lines start at its edge, whatever spaces
// the app put before them; a code block keeps its indentation, and
// slk's own look keeps the spaces.
func TestSectionLinesAreFlushInSlackLook(t *testing.T) {
	section := []Block{SectionBlock{Text: "Digest\n Trend: none\n\t Open: 2\n```\n  indented\n```\n after"}}
	ctx := previewCtx(imgpkg.ProtoOff)
	got := ansi.Strip(strings.Join(Render(section, ctx, 40).Lines, "\n"))
	if want := "Digest\nTrend: none\nOpen: 2\n```\n  indented\n```\nafter"; got != want {
		t.Errorf("Slack's look drew %q, want %q", got, want)
	}
	ctx.Slack = false
	if got := ansi.Strip(strings.Join(Render(section, ctx, 40).Lines, "\n")); !strings.Contains(got, "\n Trend: none") {
		t.Errorf("slk's own look drew %q, want the leading space kept", got)
	}
}
