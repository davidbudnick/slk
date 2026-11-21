package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// closeApp is an app with the Slack-style sidebar focused over a
// channel, a DM and a group DM. closed collects what was sent to
// ChannelService.Close, which answers with fail when that is set.
func closeApp(closed *[]string, fail error) *App {
	a := NewApp()
	a.width = 160
	a.height = 40
	a.activeTeamID = "T1"
	a.sidebarVisible = true
	a.focusedPanel = PanelSidebar
	a.sidebar.SetSlackStyle(true)
	a.SetChannels([]sidebar.ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
		{ID: "D1", Name: "Ann Lee", Type: "dm", DMUserID: "U1"},
		{ID: "G1", Name: "Bo Ito, Cy Day", Type: "group_dm", GroupSize: 2},
	})
	setChannelFuncsForTest(a, core.ChannelServiceFuncs{
		Close: func(id ids.ChannelID, name string) core.Msg {
			*closed = append(*closed, string(id))
			if fail != nil {
				return ConversationCloseFailedMsg{ChannelID: string(id), Name: name, Err: fail}
			}
			return ConversationClosedMsg{TeamID: "T1", ChannelID: string(id)}
		},
	})
	_ = a.View()
	return a
}

// sidebarHas reports whether the sidebar lists the conversation.
func sidebarHas(a *App, id string) bool {
	for _, item := range a.sidebar.Items() {
		if item.ID == id {
			return true
		}
	}
	return false
}

// sidebarLine returns the sidebar's panel row drawing name.
func sidebarLine(t *testing.T, a *App, name string) int {
	t.Helper()
	view := a.sidebar.View(a.height-3, a.sidebar.Width())
	for i, l := range strings.Split(view, "\n") {
		if strings.Contains(l, name) {
			return i
		}
	}
	t.Fatalf("sidebar does not draw %q:\n%s", name, view)
	return -1
}

// x on a DM's row asks Slack to close it, and the row goes when Slack
// answers; on a channel's row x does nothing.
func TestCloseConversation_XOnASidebarRow(t *testing.T) {
	var closed []string
	a := closeApp(&closed, nil)

	a.sidebar.SelectByID("D1")
	msgs := drainCmd(press(a, 'x'))
	if len(closed) != 1 || closed[0] != "D1" {
		t.Fatalf("Close calls = %v; want D1", closed)
	}
	if !sidebarHas(a, "D1") {
		t.Fatal("the row left before Slack answered")
	}
	for _, m := range msgs {
		a.Update(m)
	}
	if sidebarHas(a, "D1") || !sidebarHas(a, "G1") || !sidebarHas(a, "C1") {
		t.Errorf("after the close the sidebar lists %+v; want D1 alone gone", a.sidebar.Items())
	}

	a.sidebar.SelectByID("C1")
	if cmd := press(a, 'x'); cmd != nil || len(closed) != 1 {
		t.Errorf("x on a channel's row closed something: %v", closed)
	}

	// With the messages pane focused x keeps its other meaning.
	a.sidebar.SelectByID("G1")
	a.focusedPanel = PanelMessages
	_ = press(a, 'x')
	if len(closed) != 1 {
		t.Errorf("x closed a conversation with the sidebar unfocused: %v", closed)
	}
}

// A close for another workspace, or of a conversation that is not a DM,
// leaves the sidebar alone; one Slack refuses keeps the row and says so.
func TestCloseConversation_WhatStays(t *testing.T) {
	var closed []string
	a := closeApp(&closed, nil)
	a.Update(ConversationClosedMsg{TeamID: "T2", ChannelID: "D1"})
	a.Update(ConversationClosedMsg{TeamID: "T1", ChannelID: "C1"})
	if !sidebarHas(a, "D1") || !sidebarHas(a, "C1") {
		t.Errorf("sidebar lost a row it should keep: %+v", a.sidebar.Items())
	}

	a = closeApp(&closed, errors.New("channel_not_found"))
	a.sidebar.SelectByID("G1")
	for _, m := range drainCmd(press(a, 'x')) {
		a.Update(m)
	}
	if !sidebarHas(a, "G1") {
		t.Error("a refused close removed the row")
	}
	if !strings.Contains(a.statusbar.View(a.width), "Could not close Bo Ito, Cy Day") {
		t.Errorf("no toast for the refused close: %q", a.statusbar.View(a.width))
	}
}

// The pointer over a DM's row shows its close mark, and a click on the
// mark closes where a click anywhere else on the row opens it.
func TestCloseConversation_ClickOnTheMark(t *testing.T) {
	var closed []string
	a := closeApp(&closed, nil)
	if v := a.View(); v.MouseMode != tea.MouseModeAllMotion {
		t.Fatalf("mouse mode = %v; the Slack look needs every move reported", v.MouseMode)
	}
	line := sidebarLine(t, a, "Ann Lee")
	left := a.layout.RailWidth() + 1
	markX := left + a.sidebar.Width() - 2 - 3

	a.Update(tea.MouseMotionMsg{X: left + 6, Y: line + 1})
	if !strings.Contains(a.sidebar.View(a.height-3, a.sidebar.Width()), "✕") {
		t.Fatal("no close mark with the pointer over the DM's row")
	}

	msgs := drainCmd(reduceMouseClick(a, tea.MouseClickMsg{Button: tea.MouseLeft, X: left + 6, Y: line + 1}))
	if len(closed) != 0 {
		t.Fatalf("a click on the row's name closed it: %v", closed)
	}
	if sel, ok := firstMsg(msgs).(ChannelSelectedMsg); !ok || sel.ID != "D1" {
		t.Errorf("a click on the row's name produced %#v; want it opened", firstMsg(msgs))
	}

	msgs = drainCmd(reduceMouseClick(a, tea.MouseClickMsg{Button: tea.MouseLeft, X: markX, Y: line + 1}))
	if len(closed) != 1 || closed[0] != "D1" {
		t.Fatalf("a click on the mark sent Close calls %v; want D1", closed)
	}
	if got, ok := firstMsg(msgs).(ConversationClosedMsg); !ok || got.ChannelID != "D1" {
		t.Errorf("a click on the mark produced %#v; want the close's answer", firstMsg(msgs))
	}

	a.Update(tea.MouseMotionMsg{X: a.layout.SidebarEnd() + 10, Y: line + 1})
	if strings.Contains(a.sidebar.View(a.height-3, a.sidebar.Width()), "✕") {
		t.Error("the mark stayed after the pointer left the sidebar")
	}

	off := closeApp(&closed, nil)
	off.sidebar.SetSlackStyle(false)
	if v := off.View(); v.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("mouse mode without the Slack look = %v; want moves reported only while a button is held", v.MouseMode)
	}
}
