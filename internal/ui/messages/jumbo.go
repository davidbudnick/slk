package messages

import (
	"strings"

	emojiutil "github.com/gammons/slk/internal/emoji"
)

// jumboCells and jumboRows are an emoji's footprint in a message of nothing but emoji.
const jumboCells, jumboRows = 4, 2

// jumboLimit is the most emoji Slack draws large.
const jumboLimit = 23

// jumboEmoji draws text large when it is nothing but emoji, as Slack does; ok is false when it is not.
func jumboEmoji(text string, opts RenderSlackMarkdownOpts, width int) (string, bool) {
	l := CurrentLook()
	if !l.Slack || l.PlainEmoji || !emojiutil.ImageModeActive() || opts.PlaceCtx.Fetcher == nil {
		return "", false
	}
	var urls []string
	for _, tok := range emojiutil.ResolveEmojiToTokens(strings.TrimSpace(text), opts.Customs) {
		switch {
		case tok.Kind == emojiutil.TokenEmoji && tok.URL != "":
			urls = append(urls, tok.URL)
		case tok.Kind == emojiutil.TokenText && strings.TrimSpace(tok.Text) == "":
		default:
			return "", false
		}
	}
	if len(urls) == 0 || len(urls) > jumboLimit {
		return "", false
	}
	perRow := max(1, (width+1)/(jumboCells+1))
	var lines []string
	for start := 0; start < len(urls); start += perRow {
		rows := make([]string, jumboRows)
		for i, url := range urls[start:min(start+perRow, len(urls))] {
			placed, flush, ok := emojiutil.PlaceRows(opts.PlaceCtx, url, jumboCells, jumboRows)
			if !ok {
				return "", false
			}
			if flush != nil && opts.EmojiFlushes != nil {
				*opts.EmojiFlushes = append(*opts.EmojiFlushes, flush)
			}
			for r := range rows {
				if i > 0 {
					rows[r] += " "
				}
				rows[r] += placed[r]
				if strings.HasSuffix(placed[r], "\x1b[39m") {
					rows[r] += FgANSI()
				}
			}
		}
		lines = append(lines, rows...)
	}
	return strings.Join(lines, "\n"), true
}
