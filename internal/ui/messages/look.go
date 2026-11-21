package messages

import (
	"strings"
	"sync/atomic"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/ui/styles"
)

// AddReactionKey is the reaction hit key of the add-reaction pill.
const AddReactionKey = "\x00add"

// bareLinkLimit is where Slack cuts a bare link's address short.
const bareLinkLimit = 75

// Look is how message content is drawn where Slack and slk's own style differ.
type Look struct {
	Slack          bool   // links, previews, pills and separators as Slack draws them
	UnderlineLinks bool   // Slack's "underline links" preference
	PlainEmoji     bool   // Slack's jumbo emoji preference is off
	LinkGlyph      string // drawn before a bare link; "" for none
}

var look atomic.Pointer[Look]

// SetLook installs the look every pane draws message content with.
func SetLook(l Look) { look.Store(&l) }

// CurrentLook returns the installed look, slk's own when none was set.
func CurrentLook() Look {
	if l := look.Load(); l != nil {
		return *l
	}
	return Look{}
}

// TimestampStyle is the style of a message's time: upright in Slack's look.
func TimestampStyle() lipgloss.Style {
	if CurrentLook().Slack {
		return styles.Timestamp.Italic(false)
	}
	return styles.Timestamp
}

// DaySeparator draws a day's label: on a rule across the pane in Slack's look.
func DaySeparator(label string, width int) string {
	if !CurrentLook().Slack {
		return DividerStyle(width).Render("── " + label + " ──")
	}
	rule := lipgloss.NewStyle().Background(styles.Background).Foreground(styles.Border)
	name := lipgloss.NewStyle().Background(styles.Background).Foreground(styles.TextPrimary).Bold(true)
	text := " " + label + " "
	left := max(0, (width-lipgloss.Width(text))/2)
	right := max(0, width-lipgloss.Width(text)-left)
	return rule.Render(strings.Repeat("─", left)) + name.Render(text) + rule.Render(strings.Repeat("─", right))
}

// NewSeparator draws the line over the first unread message: Slack's ends in "New".
func NewSeparator(width int) string {
	style := lipgloss.NewStyle().Background(styles.Background).Foreground(styles.Error).Bold(true)
	if !CurrentLook().Slack {
		return style.Width(width).Align(lipgloss.Center).Render("── new ──")
	}
	label := " New "
	return style.Render(strings.Repeat("─", max(0, width-lipgloss.Width(label))) + label)
}

// bareLinkLabel is what a link with no label of its own shows.
func bareLinkLabel(url string) string {
	visible := strings.TrimPrefix(url, "mailto:")
	l := CurrentLook()
	if !l.Slack || visible != url {
		return visible
	}
	visible = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(visible, "https://"), "http://"), "/")
	if r := []rune(visible); len(r) > bareLinkLimit {
		visible = string(r[:bareLinkLimit]) + "…"
	}
	if l.LinkGlyph == "" {
		return visible
	}
	return l.LinkGlyph + " " + visible
}

// bareAddress is the address to draw as a bare link when label only
// repeats url: the label as written, or url when the label was cut short
// with an ellipsis as Slack's composer cuts a long one.
func bareAddress(url, label string) (string, bool) {
	if !CurrentLook().Slack {
		return "", false
	}
	trim := func(s string) string {
		s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
		return strings.TrimSuffix(strings.TrimPrefix(s, "www."), "/")
	}
	address, shown := trim(url), trim(label)
	for _, ellipsis := range []string{"…", "..."} {
		if start, cut := strings.CutSuffix(shown, ellipsis); cut {
			return url, start != "" && strings.HasPrefix(address, start)
		}
	}
	return label, shown == address
}
