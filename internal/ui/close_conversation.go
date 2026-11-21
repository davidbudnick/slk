package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// closeConversation asks Slack to close a DM, group DM or app's DM, as
// the "x" on its row in Slack's sidebar does. The row leaves the
// sidebar when Slack answers, here and on the user's other clients.
func (a *App) closeConversation(item sidebar.ChannelItem) tea.Cmd {
	if a.channels == nil || !sidebar.Closable(item) {
		return nil
	}
	channels := a.channels
	id, name := ids.ChannelID(item.ID), item.Name
	return func() tea.Msg {
		return channels.Close(id, name)
	}
}

// closeSelectedConversation is x in the sidebar: close the
// conversation under the cursor, if it is one Slack can close.
func (a *App) closeSelectedConversation() tea.Cmd {
	item, ok := a.sidebar.SelectedItem()
	if !ok {
		return nil
	}
	return a.closeConversation(item)
}

// removeClosedConversation takes a closed conversation's row out of
// the sidebar. The conversation on screen stays up, and the finder
// still offers it.
func (a *App) removeClosedConversation(channelID string) {
	items := a.sidebar.Items()
	kept := make([]sidebar.ChannelItem, 0, len(items))
	for _, item := range items {
		if item.ID == channelID && sidebar.Closable(item) {
			continue
		}
		kept = append(kept, item)
	}
	if len(kept) != len(items) {
		a.SetChannels(kept)
	}
}

// sidebarRowAt returns the sidebar's panel row at screen cell (x, y),
// or -1 when the cell is not in the sidebar's rows.
func (a *App) sidebarRowAt(x, y int) int {
	if a.bootstrap.IsLoading() || a.mode.IsModalOverlay() || !a.sidebarVisible ||
		x < a.layout.RailWidth() || x >= a.layout.SidebarEnd() || y < 1 || y >= a.height-1 {
		return -1
	}
	return y - 1
}

// reduceSidebarHover tells the sidebar which of its rows the pointer
// is over, from the moves a terminal reports with no button held, so a
// DM's row can show its close mark. Nothing else acts on those moves.
var reduceSidebarHover reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(tea.MouseMotionMsg)
	if !ok || m.Button != tea.MouseNone {
		return nil, false
	}
	a.sidebar.SetHover(a.sidebarRowAt(m.X, m.Y))
	return nil, true
}
