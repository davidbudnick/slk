// Package commandpicker is the composer's "/" menu: the workspace's
// slash commands, each with its usage and, on a second line, where it
// comes from and what it does, as Slack's own menu shows them.
//
// The picker pulls its list from a SourceFunc each time it opens, so
// it holds no copy of the workspace's commands between uses.
package commandpicker

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/bubbles/chrome"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/text"
)

var _ chrome.Dialog = (*Model)(nil)

// MaxVisible is how many commands the menu shows at once; each takes
// two lines. A longer list scrolls.
const MaxVisible = 5

// SourceFunc returns the commands to offer. It must not block.
type SourceFunc func() []core.SlashCommand

// Styles is everything the picker draws with.
type Styles struct {
	// Chrome is the shared popup styling (box, marker, scrollbar).
	Chrome chrome.Styles
	// Name styles "/name"; the selected row's is also bold.
	Name lipgloss.Style
	// Usage styles the argument hint after the name.
	Usage lipgloss.Style
	// Detail styles a row's second line.
	Detail lipgloss.Style
}

// Option configures a Model at construction.
type Option func(*Model)

// WithStyles sets the initial styles.
func WithStyles(s Styles) Option { return func(m *Model) { m.styles = s } }

// WithWidth sets the popup width.
func WithWidth(w int) Option { return func(m *Model) { m.width = w } }

// Model is the menu's state: the commands it opened with, the ones the
// query leaves, and which of those is selected.
type Model struct {
	source   SourceFunc
	commands []core.SlashCommand
	filtered []core.SlashCommand
	query    string
	selected int
	visible  bool
	styles   Styles
	width    int
}

// New returns a closed picker reading its commands from source.
func New(source SourceFunc, opts ...Option) Model {
	m := Model{source: source}
	for _, o := range opts {
		o(&m)
	}
	return m
}

// SetSource replaces where the picker reads its commands from.
func (m *Model) SetSource(source SourceFunc) { m.source = source }

// SetStyles replaces the picker's styles.
func (m *Model) SetStyles(s Styles) { m.styles = s }

// SetWidth sets the popup width the composer allots.
func (m *Model) SetWidth(w int) { m.width = w }

// Open reads the commands and shows the menu. It reports false, and
// stays closed, when there are none to show.
func (m *Model) Open() bool {
	m.Close()
	if m.source == nil {
		return false
	}
	for _, c := range m.source() {
		if c.AliasOf == "" && strings.HasPrefix(c.Name, "/") {
			m.commands = append(m.commands, c)
		}
	}
	if len(m.commands) == 0 {
		return false
	}
	sort.SliceStable(m.commands, func(i, j int) bool {
		a, b := m.commands[i], m.commands[j]
		if ra, rb := kindRank(a.Type), kindRank(b.Type); ra != rb {
			return ra < rb
		}
		return text.Fold(a.Name) < text.Fold(b.Name)
	})
	m.visible = true
	m.filter()
	return true
}

// Close hides the menu and drops its commands.
func (m *Model) Close() {
	m.visible = false
	m.query = ""
	m.selected = 0
	m.commands = nil
	m.filtered = nil
}

// IsVisible reports whether the menu is open, with or without matches.
func (m *Model) IsVisible() bool { return m.visible }

// HasMatches reports whether the open menu has a row to select.
func (m *Model) HasMatches() bool { return m.visible && len(m.filtered) > 0 }

// SetQuery filters by the text typed after the slash.
func (m *Model) SetQuery(q string) {
	if q == m.query {
		return
	}
	m.query = q
	m.selected = 0
	m.filter()
}

// Query returns the current filter text.
func (m *Model) Query() string { return m.query }

// Filtered returns the commands the query leaves, in menu order.
func (m *Model) Filtered() []core.SlashCommand { return m.filtered }

// Selected returns the selected row's index into Filtered.
func (m *Model) Selected() int { return m.selected }

// MoveUp selects the row above.
func (m *Model) MoveUp() {
	if m.selected > 0 {
		m.selected--
	}
}

// MoveDown selects the row below.
func (m *Model) MoveDown() {
	if m.selected < len(m.filtered)-1 {
		m.selected++
	}
}

// Select returns the selected command.
func (m *Model) Select() (core.SlashCommand, bool) {
	if m.selected < 0 || m.selected >= len(m.filtered) {
		return core.SlashCommand{}, false
	}
	return m.filtered[m.selected], true
}

// kindRank orders an empty query's list: slk's own commands, Slack's,
// then apps'.
func kindRank(kind string) int {
	switch kind {
	case core.SlashCommandLocal:
		return 0
	case core.SlashCommandCore:
		return 1
	}
	return 2
}

// filter keeps the commands whose name starts with the query, then
// those that only contain it, as Slack's menu does ("/ha" finds
// /hangout, then /soc-handover).
func (m *Model) filter() {
	q := text.Fold(strings.TrimPrefix(m.query, "/"))
	if q == "" {
		m.filtered = m.commands
		return
	}
	var prefix, inside []core.SlashCommand
	for _, c := range m.commands {
		name := text.Fold(strings.TrimPrefix(c.Name, "/"))
		switch {
		case strings.HasPrefix(name, q):
			prefix = append(prefix, c)
		case strings.Contains(name, q):
			inside = append(inside, c)
		}
	}
	byName := func(list []core.SlashCommand) {
		sort.SliceStable(list, func(i, j int) bool { return text.Fold(list[i].Name) < text.Fold(list[j].Name) })
	}
	byName(prefix)
	byName(inside)
	m.filtered = append(prefix, inside...)
}

// detail is a row's second line: where the command comes from, then
// what it does.
func detail(c core.SlashCommand) string {
	var parts []string
	switch c.Type {
	case core.SlashCommandCore:
		parts = append(parts, "Slack")
	case core.SlashCommandLocal:
		parts = append(parts, "slk")
	case core.SlashCommandService:
		parts = append(parts, "Command")
	default:
		parts = append(parts, "App")
	}
	if c.AppName != "" {
		parts = append(parts, c.AppName)
	}
	if c.Desc != "" {
		parts = append(parts, c.Desc)
	}
	return strings.Join(parts, " · ")
}

// View renders the popup at the width set by SetWidth/WithWidth, or ""
// when the picker is hidden or has no matches.
func (m *Model) View() string {
	if !m.visible || len(m.filtered) == 0 {
		return ""
	}
	s := m.styles.Chrome
	start, end := chrome.Window(m.selected, len(m.filtered), MaxVisible)
	list := chrome.List{Start: start, End: end, Total: len(m.filtered)}
	content := m.width - 2 - s.Popup.GetHorizontalFrameSize() - lipgloss.Width(chrome.PopupMarker(s, false))
	if list.Scrolls() {
		content--
	}
	content = max(content, 1)

	rows := make([]string, 0, 2*(end-start))
	for i := start; i < end; i++ {
		c := m.filtered[i]
		selected := i == m.selected
		nameStyle := m.styles.Name
		if selected {
			nameStyle = nameStyle.Bold(true)
		}
		name := ansi.Truncate(c.Name, content, "…")
		head := nameStyle.Render(name)
		if room := content - lipgloss.Width(name) - 1; c.Usage != "" && room > 1 {
			head += m.styles.Usage.Render(" " + ansi.Truncate(c.Usage, room, "…"))
		}
		tail := m.styles.Detail.Render(ansi.Truncate(detail(c), content, "…"))
		for _, line := range []string{head, tail} {
			pad := strings.Repeat(" ", max(content-lipgloss.Width(line), 0))
			rows = append(rows, chrome.PopupMarker(s, selected)+line+pad+list.Gutter(s, i))
		}
	}
	return chrome.Popup(s, rows, m.width)
}
