package blockkit

import (
	"image"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/styles"
)

// previewMaxWidth caps a link preview card, as Slack caps its own.
const previewMaxWidth = 78

// previewMinWidth is the narrowest pane a card is drawn in.
const previewMinWidth = 30

// previewTextRows caps the rows of a preview's description.
const previewTextRows = 3

// previewThumbRows is the height of a preview's image.
const previewThumbRows = 4

// now is the clock footers are dated against.
var now = time.Now

// isLinkPreview reports whether a is the plain preview Slack makes of a pasted link.
func isLinkPreview(a LegacyAttachment) bool {
	return (a.OriginalURL != "" || a.FromURL != "") && a.Title != "" && a.Color == "" &&
		a.AuthorName == "" && a.Pretext == "" && a.Footer == "" &&
		len(a.Blocks) == 0 && len(a.Fields) == 0 && len(a.Actions) == 0
}

// previewSite is the site named under a preview's text.
func previewSite(a LegacyAttachment) string {
	if a.ServiceName != "" {
		return a.ServiceName
	}
	raw := a.OriginalURL
	if raw == "" {
		raw = a.FromURL
	}
	if u, err := url.Parse(raw); err == nil {
		return u.Host
	}
	return ""
}

// previewThumb is the URL and cell size of a preview's image, zero when it has none to draw.
func previewThumb(a LegacyAttachment, ctx Context, maxCols int) (string, image.Point) {
	src, aspect := a.ThumbURL, 1.0
	if src == "" {
		src = a.ImageURL
		if a.ImageWidth > 0 && a.ImageHeight > 0 {
			aspect = float64(a.ImageWidth) / float64(a.ImageHeight)
		}
	}
	if src == "" || ctx.Fetcher == nil || ctx.Protocol == imgpkg.ProtoOff || ctx.CellPixels.X <= 0 || ctx.CellPixels.Y <= 0 {
		return "", image.Point{}
	}
	cellRatio := float64(ctx.CellPixels.X) / float64(ctx.CellPixels.Y)
	rows := previewThumbRows
	cols := int(math.Round(float64(rows) * aspect / cellRatio))
	if cols > maxCols {
		cols = maxCols
		rows = int(float64(cols) * cellRatio / aspect)
	}
	if cols < 1 || rows < 1 {
		return "", image.Point{}
	}
	return src, image.Pt(cols, rows)
}

// previewText is a preview's title, description and site, each row at most width wide.
func previewText(a LegacyAttachment, ctx Context, width int) []string {
	title := truncateToWidth(a.Title, width)
	if a.TitleLink != "" {
		title = "\x1b]8;;" + a.TitleLink + "\x1b\\" + title + "\x1b]8;;\x1b\\"
	}
	rows := []string{lipgloss.NewStyle().Bold(true).Render(title)}
	if a.Text != "" {
		body := renderTextLines(a.Text, ctx, width)
		if len(body) > previewTextRows {
			body = body[:previewTextRows]
			body[previewTextRows-1] = ansi.Truncate(body[previewTextRows-1], width-1, "") + "…"
		}
		rows = append(rows, body...)
	}
	if site := previewSite(a); site != "" {
		rows = append(rows, mutedStyle().Render(truncateToWidth(site, width)))
	}
	return rows
}

// appendLinkPreview draws a as Slack's preview card: title, text and site in a frame, the image at their right.
func appendLinkPreview(out *RenderResult, a LegacyAttachment, ctx Context, width int) {
	cardW := min(width, previewMaxWidth)
	innerW := cardW - 4
	src, thumb := previewThumb(a, ctx, innerW/3)
	var thumbLines []string
	var thumbSixel map[int]SixelEntry
	if thumb.X > 0 {
		lines, flushes, sxl, _, ok := fetchOrPlaceholder(src, thumb, ctx, 0)
		if ok {
			thumbLines, thumbSixel = lines, sxl
			out.Flushes = append(out.Flushes, flushes...)
		} else {
			thumb = image.Point{}
		}
	}
	textW, thumbCol := innerW, 0
	if thumb.X > 0 {
		textW = innerW - thumb.X - 2
		thumbCol = 2 + textW + 2
	}
	text := previewText(a, ctx, textW)

	frame := lipgloss.NewStyle().Foreground(styles.Border).Background(styles.Background)
	side := frame.Render("│")
	fg := ansi.Style{}.ForegroundColor(styles.TextPrimary).String()
	startRow := len(out.Lines)
	out.Lines = append(out.Lines, frame.Render("╭"+strings.Repeat("─", cardW-2)+"╮"))
	for i := range max(len(text), len(thumbLines)) {
		row := ""
		if i < len(text) {
			row = text[i]
		}
		row = padRight(row, textW)
		if thumb.X > 0 {
			cells := strings.Repeat(" ", thumb.X)
			if i < len(thumbLines) {
				cells = thumbLines[i]
			}
			row += "  " + cells
		}
		out.Lines = append(out.Lines, side+" "+fg+row+" "+side)
	}
	out.Lines = append(out.Lines, frame.Render("╰"+strings.Repeat("─", cardW-2)+"╯"))

	if thumb.X > 0 {
		out.Hits = append(out.Hits, HitRect{
			RowStart: startRow + 1, RowEnd: startRow + 1 + thumb.Y,
			ColStart: thumbCol, ColEnd: thumbCol + thumb.X,
		})
	}
	for k, v := range thumbSixel {
		if out.SixelRows == nil {
			out.SixelRows = map[int]SixelEntry{}
		}
		v.Col = thumbCol
		out.SixelRows[startRow+1+k] = v
	}
}

// footerTime dates a footer as Slack does: "Yesterday at 12:45 PM".
func footerTime(ts int64, at time.Time) string {
	t := time.Unix(ts, 0).In(at.Location())
	clock := " at " + t.Format("3:04 PM")
	sameDay := func(a, b time.Time) bool {
		return a.Year() == b.Year() && a.YearDay() == b.YearDay()
	}
	switch {
	case sameDay(t, at):
		return "Today" + clock
	case sameDay(t, at.AddDate(0, 0, -1)):
		return "Yesterday" + clock
	}
	day := t.Format("Jan ") + strconv.Itoa(t.Day()) + ordinal(t.Day())
	if t.Year() != at.Year() {
		day += t.Format(", 2006")
	}
	return day + clock
}

// ordinal is the suffix of a day of the month: "st", "nd", "rd" or "th".
func ordinal(day int) string {
	if day%100 >= 11 && day%100 <= 13 {
		return "th"
	}
	switch day % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}
