package compose

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/bubbles/commandpicker"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

// SetCommandSource sets where the "/" menu reads the workspace's slash
// commands from. It is asked each time the menu opens.
func (m *Model) SetCommandSource(source commandpicker.SourceFunc) {
	m.commandPicker.SetSource(source)
}

// openCommandOnSlash opens the "/" menu when the key just typed made
// the composer's whole text a lone slash, as in Slack: a slash at
// position 0 of an empty composer. With no commands to show it stays
// closed.
func (m *Model) openCommandOnSlash(key tea.KeyMsg) {
	if key.Key().Text == "/" && m.input.Value() == "/" {
		m.commandActive = m.commandPicker.Open()
	}
}

// syncCommand keeps the open menu's filter on the text after the slash,
// and ends the session once the text is no longer a command name being
// typed: the slash is gone, or a space or newline follows the name.
func (m *Model) syncCommand() {
	if !m.commandActive {
		return
	}
	val := m.input.Value()
	if !strings.HasPrefix(val, "/") || strings.ContainsAny(val, " \t\n") {
		m.CloseCommand()
		return
	}
	m.commandPicker.SetQuery(val[1:])
}

// handleCommandKey processes a key while the "/" menu shows matches.
// Enter and Tab complete the selected command and never run one.
func (m Model) handleCommandKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	k := msg.Key()
	switch {
	case k.Code == tea.KeyUp || (k.Code == 'p' && isCtrl(k.Mod)):
		m.commandPicker.MoveUp()
		return m, nil

	case k.Code == tea.KeyDown || (k.Code == 'n' && isCtrl(k.Mod)):
		m.commandPicker.MoveDown()
		return m, nil

	case k.Code == tea.KeyEnter || k.Code == tea.KeyTab:
		if c, ok := m.commandPicker.Select(); ok {
			m.input.SetValue(c.Name + " ")
			m.input.CursorEnd()
		}
		m.CloseCommand()
		m.autoGrow()
		return m, nil

	case k.Code == tea.KeyEscape:
		m.CloseCommand()
		return m, nil

	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.syncCommand()
		m.autoGrow()
		return m, cmd
	}
}

// IsCommandActive returns whether the "/" menu is showing. A session
// whose text matches no command shows nothing and takes no keys, so
// Enter then runs or sends what was typed.
func (m Model) IsCommandActive() bool {
	return m.commandActive && m.commandPicker.HasMatches()
}

// CloseCommand dismisses the "/" menu without selecting.
func (m *Model) CloseCommand() {
	m.commandActive = false
	m.commandPicker.Close()
}

// CommandPickerView returns the rendered "/" menu, or "" if not showing.
func (m Model) CommandPickerView(width int) string {
	if !m.IsCommandActive() {
		return ""
	}
	// m is a copy; the width is the popup width the caller allots this frame.
	m.commandPicker.SetWidth(width)
	return m.commandPicker.View()
}

// commandPickerStyles maps the current theme onto the "/" menu.
func commandPickerStyles() commandpicker.Styles {
	return commandpicker.Styles{
		Chrome: messages.ChromeStyles(),
		Name:   lipgloss.NewStyle().Foreground(styles.TextPrimary),
		Usage:  lipgloss.NewStyle().Foreground(styles.TextMuted),
		Detail: lipgloss.NewStyle().Foreground(styles.TextMuted),
	}
}

// PickerActive reports whether one of the composer's popups (emoji,
// mention, channel, command) is showing and so owns the keys.
func (m Model) PickerActive() bool {
	return m.emojiActive || m.mentionActive || m.channelActive || m.IsCommandActive()
}

// ClosePicker dismisses the showing popup and reports whether there
// was one.
func (m *Model) ClosePicker() bool {
	switch {
	case m.emojiActive:
		m.CloseEmoji()
	case m.mentionActive:
		m.CloseMention()
	case m.channelActive:
		m.CloseChannel()
	case m.IsCommandActive():
		m.CloseCommand()
	default:
		return false
	}
	return true
}

// PopupView returns the showing popup, rendered for the composer's
// width, or "". The popups are mutually exclusive in Update; emoji wins
// if somehow two are open.
func (m Model) PopupView(width int) string {
	if v := m.EmojiPickerView(width); v != "" {
		return v
	}
	if v := m.MentionPickerView(width); v != "" {
		return v
	}
	if v := m.ChannelPickerView(width); v != "" {
		return v
	}
	return m.CommandPickerView(width)
}
