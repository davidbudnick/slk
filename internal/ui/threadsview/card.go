package threadsview

import (
	"io"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/muesli/reflow/truncate"
)

// Line caps for message bodies in a card; the thread pane shows the rest.
const (
	parentMaxLines = 6
	replyMaxLines  = 4
)

// avatarCols is the avatar's width plus the gap after it.
const avatarCols = 5

// cardChromeCols is the card's border and padding on both sides.
const cardChromeCols = 4

// minCardWidth is the narrowest a card is laid out at; a narrower pane
// shows the left part of that layout.
const minCardWidth = 24

// renderCard draws one thread as Slack's Threads view does: the channel
// and its participants, then a rounded card holding the parent, a "Show N
// more replies" link and the newest replies. The selected card's border
// takes the selection colour. Every line is exactly width columns wide.
func (m *Model) renderCard(s core.ThreadSummary, width int, selected bool) ([]string, []func(io.Writer) error) {
	if width < minCardWidth {
		lines, flushes := m.renderCard(s, minCardWidth, selected)
		for i, l := range lines {
			lines[i] = ansi.Truncate(l, width, "")
		}
		return lines, flushes
	}
	var flushes []func(io.Writer) error
	innerWidth := width - cardChromeCols
	textWidth := innerWidth
	if m.avatarFn != nil {
		textWidth -= avatarCols
	}

	var blocks []string
	if s.ParentText == "" && s.ParentUserID == "" {
		blocks = append(blocks, mutedStyle().Render("(parent not loaded)"))
	} else {
		blocks = append(blocks, m.renderEntry(s.ParentUserID, s.ParentText, s.ParentTS, textWidth, parentMaxLines, &flushes))
	}
	replies := s.PreviewReplies()
	if link := m.moreRepliesLine(s, len(replies)); link != "" {
		blocks = append(blocks, clipToWidth(link, innerWidth))
	}
	for _, r := range replies {
		blocks = append(blocks, m.renderEntry(r.UserID, r.Text, r.TS, textWidth, replyMaxLines, &flushes))
	}

	border := styles.Border
	if selected {
		border = styles.SelectionBorderColor(m.focused)
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		BorderBackground(styles.Background).
		Background(styles.Background).
		Padding(0, 1).
		Width(width).
		Render(strings.Join(blocks, "\n\n"))

	lines := []string{
		fillRow(" "+m.channelTitle(s, selected), width),
	}
	if who := m.participantsLine(s); who != "" {
		lines = append(lines, fillRow(" "+mutedStyle().Render(who), width))
	}
	return append(lines, strings.Split(box, "\n")...), flushes
}

// renderEntry draws one message: the avatar, the author and when it was
// sent, and the body capped at maxLines rows with a "Show more" link
// after a cut.
func (m *Model) renderEntry(userID, text, ts string, textWidth, maxLines int, flushes *[]func(io.Writer) error) string {
	name := styles.Username(userID, m.coloredUsernames).Bold(true).Render(m.displayName(userID))
	header := name
	if when := whenLabel(ts); when != "" {
		header += bgStyle().Render("  ") + styles.Timestamp.Italic(false).Render(when)
	}
	header = clipToWidth(header, textWidth)

	body := mutedStyle().Render("(no text)")
	if strings.TrimSpace(text) != "" {
		rendered := messages.RenderSlackMarkdownWith(text, messages.RenderSlackMarkdownOpts{
			UserNames:    m.userNames,
			ChannelNames: m.channelNames,
			UserGroups:   m.userGroups,
			Self:         messages.SelfMentions{UserID: m.selfUserID},
			PlaceCtx:     m.emojiCtx.PlaceCtx,
			EmojiCells:   m.emojiCtx.Cells,
			Customs:      m.emojiCtx.Customs,
			EmojiFlushes: flushes,
			Width:        textWidth,
		})
		rows := strings.Split(strings.Trim(messages.WordWrap(rendered, textWidth), "\n"), "\n")
		if len(rows) > maxLines {
			rows = append(rows[:maxLines], linkStyle().Render("Show more"))
		}
		body = styles.MessageText.Render(strings.Join(rows, "\n"))
	}

	content := header + "\n" + body
	if m.avatarFn == nil {
		return content
	}
	avatar := m.avatarFn(userID)
	if avatar == "" {
		blank := bgStyle().Render(strings.Repeat(" ", avatarCols-1))
		avatar = blank + "\n" + blank
	}
	return messages.PlaceAvatarBeside(avatar, content)
}

// moreRepliesLine is the link between the parent and the replies shown:
// "Show N more replies" for the replies a card leaves out, or, when none
// are cached, the reply count and when the last one came.
func (m *Model) moreRepliesLine(s core.ThreadSummary, shown int) string {
	hidden := s.ReplyCount - shown
	if hidden <= 0 {
		return ""
	}
	if shown == 0 {
		line := linkStyle().Render(pluralize(hidden, "reply", "replies"))
		if when := whenLabel(s.LastReplyTS); when != "" {
			line += mutedStyle().Render("  Last reply " + lowerFirst(when))
		}
		return line
	}
	return linkStyle().Render("Show " + pluralize(hidden, "more reply", "more replies"))
}

// channelTitle is the card's heading: the channel glyph and name, the
// other people for a DM or group DM, and a dot when the thread is unread.
func (m *Model) channelTitle(s core.ThreadSummary, selected bool) string {
	name := m.channelNames[s.ChannelID]
	if name == "" {
		name = s.ChannelName
	}
	chType := s.ChannelType
	if chType == "" && strings.HasPrefix(s.ChannelID, "D") {
		// A DM the channel cache has not seen: Slack's D ids are DMs.
		chType = "dm"
	}
	isDM := chType == "dm" || chType == "group_dm"
	if isDM && (name == "" || strings.HasPrefix(name, "mpdm-")) {
		name = strings.Join(m.otherParticipants(s), ", ")
		if name == "" && m.selfIn(s) {
			name = m.displayName(m.selfUserID)
		}
	}
	if name == "" {
		name = s.ChannelID
	}
	glyphStyle := lipgloss.NewStyle().Foreground(styles.TextMuted).Background(styles.Background)
	if chType == "private" {
		glyphStyle = glyphStyle.Foreground(styles.Warning)
	}
	nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimary).Background(styles.Background).Bold(true)
	if selected && m.focused {
		nameStyle = nameStyle.Foreground(styles.Primary)
	}
	title := glyphStyle.Render(messages.ChannelGlyph(chType)) + bgStyle().Render(" ") + nameStyle.Render(name)
	if s.Unread {
		title += bgStyle().Render("  ") + lipgloss.NewStyle().Foreground(styles.Primary).Background(styles.Background).Bold(true).Render("●")
	}
	return title
}

// participantsLine names the thread's participants as Slack does under
// the channel: "Ann and Bob", "Ann, Bob and you", "Ann, Bob, Cy and 2
// others".
func (m *Model) participantsLine(s core.ThreadSummary) string {
	names := m.otherParticipants(s)
	if m.selfIn(s) {
		names = append(names, "you")
	}
	switch n := len(names); {
	case n == 0:
		return ""
	case n == 1:
		return names[0]
	case n <= 4:
		return strings.Join(names[:n-1], ", ") + " and " + names[n-1]
	default:
		return strings.Join(names[:3], ", ") + " and " + strconv.Itoa(n-3) + " others"
	}
}

// otherParticipants returns the display names of the participants other
// than the current user, in the summary's order.
func (m *Model) otherParticipants(s core.ThreadSummary) []string {
	var names []string
	for _, id := range s.ParticipantIDs() {
		if id != m.selfUserID {
			names = append(names, m.displayName(id))
		}
	}
	return names
}

// displayName maps a user ID to its display name, or the ID when unknown.
func (m *Model) displayName(uid string) string {
	if uid == "" {
		return "Unknown"
	}
	if name := m.userNames[uid]; name != "" {
		return name
	}
	return uid
}

// whenLabel formats a Slack ts as Slack's thread headers do: the clock
// for today, else "Yesterday at 3:53 PM", "Wednesday at 6:31 PM", "Nov 4
// at 2:02 PM".
func whenLabel(ts string) string {
	clock := messages.ClockFromTS(ts)
	if clock == "" {
		return ""
	}
	day := messages.FormatShortDate(messages.DateFromTS(ts))
	if day == "Today" {
		return clock
	}
	return day + " at " + clock
}

// lowerFirst lowercases a label's leading "Yesterday"/"Today" for use
// mid-sentence; weekdays and months keep their capital.
func lowerFirst(s string) string {
	if strings.HasPrefix(s, "Yesterday") {
		return "y" + s[1:]
	}
	return s
}

// pluralize renders "1 reply" / "3 replies".
func pluralize(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// selfIn reports whether the current user is one of s's participants.
func (m *Model) selfIn(s core.ThreadSummary) bool {
	return m.selfUserID != "" && slices.Contains(s.ParticipantIDs(), m.selfUserID)
}

// bgStyle is the theme background, for gaps between styled spans.
func bgStyle() lipgloss.Style { return lipgloss.NewStyle().Background(styles.Background) }

// mutedStyle is secondary text on the theme background.
func mutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.TextMuted).Background(styles.Background)
}

// linkStyle is Slack's link blue: the theme's primary colour.
func linkStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.Primary).Background(styles.Background)
}

// fillRow clips s to width and pads it with the theme background.
func fillRow(s string, width int) string {
	return bgStyle().Width(width).Render(clipToWidth(s, width))
}

// clipToWidth truncates an already-styled string to at most `width` display
// columns (using a trailing ellipsis), but does NOT pad short strings.
// lipgloss.Width measures display columns even with embedded ANSI escapes.
func clipToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	return truncate.StringWithTail(s, uint(width), "…")
}
