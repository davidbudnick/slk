package sidebar

import "github.com/charmbracelet/x/ansi"

// closeMark is drawn near the right end of a DM's row while the
// pointer is over it, like the "x" Slack shows there.
const closeMark = " ✕"

// closeMarkInset is how far from the row's right end closeMark starts:
// where a one-digit badge sits.
const closeMarkInset = 4

// Closable reports whether item is a conversation Slack can close: a
// DM, a group DM or an app's DM.
func Closable(item ChannelItem) bool {
	return item.Type == "dm" || item.Type == "group_dm" || item.Type == "app"
}

// TracksHover reports whether the sidebar draws anything for the row
// under the pointer, which costs the terminal reporting every move.
func (m *Model) TracksHover() bool {
	return m.slackStyle
}

// SetHover records the panel row the pointer is over, so that row can
// show its close mark. A negative y means the pointer is elsewhere.
func (m *Model) SetHover(y int) {
	hover := 0
	if y >= 0 && m.slackStyle {
		hover = y + m.yOffset + 1
	}
	if hover == m.hover {
		return
	}
	redraw := m.closableLine(m.hover-1) || m.closableLine(hover-1)
	m.hover = hover
	if redraw {
		m.dirty()
	}
}

// closableLine reports whether cacheRows[line] is a closable row.
func (m *Model) closableLine(line int) bool {
	return line >= 0 && line < len(m.cacheRows) && m.cacheRows[line].closable
}

// CloseAt returns the conversation whose close mark is at panel cell
// (x, y). There is one only on the row the pointer is over.
func (m *Model) CloseAt(x, y int) (ChannelItem, bool) {
	line := y + m.yOffset
	if line+1 != m.hover || !m.closableLine(line) {
		return ChannelItem{}, false
	}
	from := m.cacheWidth - 2 - closeMarkInset
	if x < from-1 || x > from+len(closeMark)-1 {
		return ChannelItem{}, false
	}
	ni := m.cacheRows[line].navIdx
	if ni < 0 || ni >= len(m.nav) || m.nav[ni].kind != navChannel {
		return ChannelItem{}, false
	}
	return m.items[m.filtered[m.nav[ni].fi]], true
}

// withCloseMark overlays closeMark on a rendered row of the given
// panel width, in the style the row has there, keeping the cells either
// side of it.
func withCloseMark(row string, width int) string {
	from := width - 2 - closeMarkInset
	if from < 0 {
		return row
	}
	// A cut replays the styles in force at its start, and ends with
	// those that follow its end: the left part's reset among them.
	style := leadingEscapes(ansi.Cut(row, from, width-2))
	return ansi.Cut(row, 0, from) + style + closeMark + ansi.Cut(row, from+2, width-2)
}

// leadingEscapes returns the CSI sequences s starts with.
func leadingEscapes(s string) string {
	i := 0
	for i+1 < len(s) && s[i] == 0x1b && s[i+1] == '[' {
		j := i + 2
		for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
			j++
		}
		if j == len(s) {
			break
		}
		i = j + 1
	}
	return s[:i]
}
