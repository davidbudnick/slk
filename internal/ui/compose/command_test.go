package compose

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
)

// commandComposer is a focused composer whose "/" menu reads three
// commands.
func commandComposer() Model {
	m := New("general")
	m.SetCommandSource(func() []core.SlashCommand {
		return []core.SlashCommand{
			{Name: "/giphy", Usage: "[text]", Desc: "Post a random gif", Type: core.SlashCommandService, AppName: "Giphy"},
			{Name: "/remind", Desc: "Set a reminder", Type: core.SlashCommandCore},
			{Name: "/shrug", Usage: "your text", Type: core.SlashCommandCore},
		}
	})
	m.SetWidth(80)
	m.Focus()
	return m
}

func key(code rune) tea.KeyMsg { return tea.KeyPressMsg{Code: code} }

// The menu opens for a slash typed at position 0 of an empty composer,
// as in Slack, and nowhere else: not after text, not in a path, not for
// a slash that arrives by paste or SetValue.
func TestCommandMenu_OpensOnlyOnASlashInAnEmptyComposer(t *testing.T) {
	m := typeText(commandComposer(), "/")
	if !m.IsCommandActive() || !m.PickerActive() {
		t.Fatal("no menu after / in an empty composer")
	}
	if view := m.PopupView(80); !strings.Contains(view, "/giphy") || !strings.Contains(view, "/shrug") {
		t.Errorf("menu does not list the commands:\n%s", view)
	}

	for _, text := range []string{"see /", "a/", " /", "x\n/"} {
		if m := typeText(commandComposer(), text); m.IsCommandActive() {
			t.Errorf("menu opened after typing %q", text)
		}
	}

	m = commandComposer()
	m.SetValue("/")
	m, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if m.IsCommandActive() {
		t.Error("menu opened for a slash that was not typed")
	}

	none := New("general")
	none.SetWidth(80)
	none.Focus()
	if none = typeText(none, "/"); none.IsCommandActive() || none.PickerActive() {
		t.Error("menu opened with no commands to show")
	}
}

// Typing filters; a name that matches nothing hides the menu and gives
// the keys back, so Enter then runs what was typed; deleting brings the
// rows back; a space ends the session.
func TestCommandMenu_FollowsTheNameBeingTyped(t *testing.T) {
	m := typeText(commandComposer(), "/re")
	if view := m.PopupView(80); !strings.Contains(view, "/remind") || strings.Contains(view, "/giphy") {
		t.Errorf("/re does not filter to /remind:\n%s", view)
	}

	m = typeText(m, "x")
	if m.IsCommandActive() || m.PickerActive() || m.PopupView(80) != "" {
		t.Error("menu still owns the keys for /rex, which matches nothing")
	}
	m, _ = m.Update(key(tea.KeyBackspace))
	if !m.IsCommandActive() {
		t.Error("deleting the letter that matched nothing did not bring the menu back")
	}

	m = typeText(m, "mind ")
	if m.IsCommandActive() {
		t.Error("menu still open after the space that ends the command's name")
	}
	if m = typeText(m, "list"); m.IsCommandActive() || m.Value() != "/remind list" {
		t.Errorf("after the name: active=%v value=%q; want the menu closed and the text as typed", m.IsCommandActive(), m.Value())
	}

	m = typeText(commandComposer(), "/")
	m, _ = m.Update(key(tea.KeyBackspace))
	if m.IsCommandActive() || m.Value() != "" {
		t.Errorf("after deleting the slash: active=%v value=%q", m.IsCommandActive(), m.Value())
	}
	if m = typeText(m, "hi"); m.PickerActive() {
		t.Error("a closed menu came back for ordinary text")
	}
}

// Enter and Tab complete the selected command and leave it for the
// user to finish; neither sends it. Esc closes and keeps the text.
func TestCommandMenu_CompletesAndCloses(t *testing.T) {
	for name, code := range map[string]rune{"Enter": tea.KeyEnter, "Tab": tea.KeyTab} {
		m := typeText(commandComposer(), "/")
		m, _ = m.Update(key(tea.KeyDown))
		m, _ = m.Update(key(tea.KeyDown))
		m, _ = m.Update(key(tea.KeyUp))
		m, _ = m.Update(key(code))
		if m.Value() != "/shrug " {
			t.Errorf("%s completed to %q; want %q", name, m.Value(), "/shrug ")
		}
		if m.IsCommandActive() {
			t.Errorf("%s left the menu open", name)
		}
		if m = typeText(m, "ok"); m.Value() != "/shrug ok" {
			t.Errorf("typing after %s gives %q; want the cursor after the completed name", name, m.Value())
		}
	}

	m := typeText(commandComposer(), "/gi")
	m, _ = m.Update(key(tea.KeyEscape))
	if m.IsCommandActive() || m.Value() != "/gi" {
		t.Errorf("after Esc: active=%v value=%q; want closed with the text kept", m.IsCommandActive(), m.Value())
	}

	m = typeText(commandComposer(), "/gi")
	if !m.ClosePicker() || m.PickerActive() {
		t.Error("ClosePicker did not close the menu")
	}
	if m.ClosePicker() {
		t.Error("ClosePicker reported a popup with none open")
	}

	m = typeText(commandComposer(), "/gi")
	m.Reset()
	if m.IsCommandActive() {
		t.Error("the menu survived the composer being cleared")
	}
}
