package messages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// filterURL is an address whose "~", "*" and "_" would each read as markup.
const filterURL = "https://app.example/p/issues#~(filters~(status~(equals~(~'OPEN))~_sev_~*high*))"

// An address is never read as markup: the link draws as its label and opens the whole address.
func TestLinkAddressIsNotMarkup(t *testing.T) {
	for _, look := range []Look{{}, {Slack: true}, {Slack: true, UnderlineLinks: true}} {
		withLook(t, look)
		src := "Open: <" + filterURL + "|*0* Critical> | <" + filterURL + "|*12* Medium> ~gone~"
		got := RenderSlackMarkdown(src, nil, nil)
		if plain := ansi.Strip(got); plain != "Open: 0 Critical | 12 Medium gone" {
			t.Errorf("look %+v drew %q", look, plain)
		}
		if n := strings.Count(got, "\x1b]8;;"+filterURL+"\x1b\\"); n != 2 {
			t.Errorf("look %+v: %d links open the whole address, want 2", look, n)
		}
	}
}

// A bare link whose address holds markup characters draws them as written.
func TestBareLinkKeepsItsMarkupCharacters(t *testing.T) {
	withLook(t, Look{Slack: true})
	got := ansi.Strip(RenderSlackMarkdown("<https://a.example/~sam/_notes_/*>", nil, nil))
	if got != "a.example/~sam/_notes_/*" {
		t.Errorf("drew %q", got)
	}
}

// A label's own bold, italic, struck and code spans are drawn inside the link, in the link's colour.
func TestLinkLabelKeepsItsSpans(t *testing.T) {
	withLook(t, Look{Slack: true})
	link := linkStyle()
	for label, want := range map[string]string{
		"*0* Critical":    link.Bold(true).Render("0") + link.Render(" Critical"),
		"_new_ ~old~":     link.Italic(true).Render("new") + link.Render(" ") + link.Strikethrough(true).Render("old"),
		"run `make` now":  link.Render("run ") + codeStyle().Render("make") + link.Render(" now"),
		"snake_case_name": link.Render("snake_case_name"),
		"*_both_*":        link.Bold(true).Italic(true).Render("both"),
	} {
		if got := drawSpans(label, link); got != want {
			t.Errorf("%q drew %q, want %q", label, got, want)
		}
	}
}

// Emphasis around a link still reaches it, and a struck link keeps its address whole.
func TestEmphasisAroundALink(t *testing.T) {
	withLook(t, Look{Slack: true})
	for src, want := range map[string]string{
		"*<https://a.example/x|Title>* and _<https://a.example/y|more>_": "Title and more",
		"~<https://a.example/x|old>~ `<https://a.example/y|code>`":       "old code",
	} {
		got := RenderSlackMarkdown(src, nil, nil)
		if plain := ansi.Strip(got); plain != want {
			t.Errorf("%q drew %q, want %q", src, plain, want)
		}
		if !strings.Contains(got, "\x1b]8;;https://a.example/x\x1b\\") {
			t.Errorf("%q lost its link: %q", src, got)
		}
	}
}
