package messages

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// linkStandIn is the first of the private-use runes that stand in for
// links while the emphasis passes run.
const linkStandIn = '\U00100000'

// labelItalicRe is an _italic_ candidate; italicSpan checks its word boundaries.
var labelItalicRe = regexp.MustCompile(`_([^_\n]+)_`)

// labelMarks are the spans a link's label can mark, and what each adds
// to the link's style.
var labelMarks = []struct {
	find  func(string) []int
	style func(lipgloss.Style) lipgloss.Style
}{
	{inlineCodeRe.FindStringSubmatchIndex, func(lipgloss.Style) lipgloss.Style { return codeStyle() }},
	{boldRe.FindStringSubmatchIndex, func(s lipgloss.Style) lipgloss.Style { return s.Bold(true) }},
	{italicSpan, func(s lipgloss.Style) lipgloss.Style { return s.Italic(true) }},
	{strikethroughRe.FindStringSubmatchIndex, func(s lipgloss.Style) lipgloss.Style { return s.Strikethrough(true) }},
}

// stashLinks swaps each link in text for a one-rune stand-in and returns
// the links as drawn: <url|label> as its label and <url> as its address,
// each an OSC 8 hyperlink so a terminal can open it.
func stashLinks(text string) (string, []string) {
	if !strings.ContainsRune(text, '<') {
		return text, nil
	}
	var drawn []string
	stash := func(url, label string) string {
		drawn = append(drawn, osc8Hyperlink(url, label))
		return string(linkStandIn + rune(len(drawn)-1))
	}
	text = linkWithLabelRe.ReplaceAllStringFunc(text, func(match string) string {
		parts := linkWithLabelRe.FindStringSubmatch(match)
		url, label := parts[1], slackEntityDecoder.Replace(parts[2])
		if address, bare := bareAddress(url, label); bare {
			return stash(url, linkStyle().Render(bareLinkLabel(address)))
		}
		return stash(url, drawSpans(label, linkStyle()))
	})
	text = linkBareRe.ReplaceAllStringFunc(text, func(match string) string {
		url := linkBareRe.FindStringSubmatch(match)[1]
		return stash(url, linkStyle().Render(bareLinkLabel(url)))
	})
	return text, drawn
}

// restoreLinks puts each drawn link back where stashLinks left its stand-in.
func restoreLinks(text string, drawn []string) string {
	if len(drawn) == 0 {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if i := int(r - linkStandIn); r >= linkStandIn && i < len(drawn) {
			b.WriteString(drawn[i])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// drawSpans draws text in base, and each span text marks as bold,
// italic, struck or code in base with that added.
func drawSpans(text string, base lipgloss.Style) string {
	if text == "" {
		return ""
	}
	for _, m := range labelMarks {
		if loc := m.find(text); loc != nil {
			return drawSpans(text[:loc[0]], base) + drawSpans(text[loc[2]:loc[3]], m.style(base)) + drawSpans(text[loc[1]:], base)
		}
	}
	return base.Render(text)
}

// italicSpan finds text's first _italic_ span under renderItalics' rule
// that an underscore inside a word is not a mark.
func italicSpan(text string) []int {
	for from := 0; from < len(text); {
		loc := labelItalicRe.FindStringSubmatchIndex(text[from:])
		if loc == nil {
			return nil
		}
		start, end := from+loc[0], from+loc[1]
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		after, _ := utf8.DecodeRuneInString(text[end:])
		if (start == 0 || !isItalicWordRune(before)) && (end == len(text) || !isItalicWordRune(after)) {
			return []int{start, end, from + loc[2], from + loc[3]}
		}
		from = start + 1
	}
	return nil
}
