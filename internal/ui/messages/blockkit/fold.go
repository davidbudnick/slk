package blockkit

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/ui/styles"
)

// foldLines and foldChars are how much of an attachment's text Slack
// shows before "Show more": five lines, blank ones aside, or 700 characters.
const foldLines, foldChars = 5, 700

// showMore and showLess label the row that unfolds and folds an attachment's text.
const showMore, showLess = "Show more", "Show less"

// foldText returns the part of text Slack shows before "Show more", and
// whether it left any out.
func foldText(text string) (string, bool) {
	text = strings.TrimRight(text, "\n")
	end, lines := len(text), 0
	for i := 0; i < len(text); {
		nl := strings.IndexByte(text[i:], '\n')
		if nl < 0 {
			break
		}
		lines++
		if lines == foldLines {
			end = i + nl
			break
		}
		i += nl
		for i < len(text) && text[i] == '\n' {
			i++
		}
	}
	shown := text[:end]
	if utf8.RuneCountInString(shown) <= foldChars {
		return shown, end < len(text)
	}
	cut := 0
	for i := range shown {
		if cut++; cut > foldChars {
			shown = shown[:i]
			break
		}
	}
	if sp := strings.LastIndexFunc(shown, unicode.IsSpace); sp > 0 {
		shown = shown[:sp]
	}
	if open := strings.LastIndexByte(shown, '<'); open >= 0 && !strings.Contains(shown[open:], ">") {
		shown = shown[:open]
	}
	return strings.TrimRightFunc(shown, unicode.IsSpace) + "…", true
}

// HasFold reports whether any of atts has text Slack folds behind "Show more".
func HasFold(atts []LegacyAttachment) bool {
	for _, a := range atts {
		if _, folded := foldText(a.Text); folded && !isLinkPreview(a) {
			return true
		}
	}
	return false
}

// foldRow draws the row that unfolds an attachment's text, or folds it again.
func foldRow(unfolded bool) string {
	label := showMore
	if unfolded {
		label = showLess
	}
	return lipgloss.NewStyle().Foreground(styles.Primary).Background(styles.Background).Render(label)
}
