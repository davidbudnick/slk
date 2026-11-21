package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// leaveChannelMsg is the answer to the prompt /leave raises on a private
// channel: leave it. TeamID is the workspace the command was typed in.
type leaveChannelMsg struct {
	TeamID string
	Item   sidebar.ChannelItem
}

// sidebarItem returns the sidebar's row for a conversation.
func (a *App) sidebarItem(channelID string) (sidebar.ChannelItem, bool) {
	for _, item := range a.sidebar.Items() {
		if item.ID == channelID {
			return item, true
		}
	}
	return sidebar.ChannelItem{}, false
}

// leaveConversation asks Slack to take the user out of item, as /leave
// does: a channel is left, and a DM, group DM or app's DM is closed.
// The row goes, and the view moves off the conversation, when Slack
// answers.
func (a *App) leaveConversation(item sidebar.ChannelItem) tea.Cmd {
	if a.channels == nil {
		return nil
	}
	a.leaving = item.ID
	if sidebar.Closable(item) {
		return a.closeConversation(item)
	}
	channels := a.channels
	id, name := ids.ChannelID(item.ID), item.Name
	return func() tea.Msg {
		return channels.Leave(id, name)
	}
}

// removeLeftChannel takes a channel the user left out of the sidebar,
// and leaves the finder offering a public one to join again.
func (a *App) removeLeftChannel(channelID string) {
	items := a.sidebar.Items()
	kept := make([]sidebar.ChannelItem, 0, len(items))
	for _, item := range items {
		if item.ID == channelID && !sidebar.Closable(item) {
			continue
		}
		kept = append(kept, item)
	}
	if len(kept) != len(items) {
		a.SetChannels(kept)
		a.channelFinder.MarkLeft(channelID)
	}
}

// moveOffLeft moves the view off a conversation /leave took the user
// out of: back to the one before it, or to the sidebar's selection when
// there was none. A conversation left or closed on another client stays
// on screen.
func (a *App) moveOffLeft(channelID string) tea.Cmd {
	if a.leaving != channelID {
		return nil
	}
	a.leaving = ""
	if a.activeChannelID != channelID {
		return nil
	}
	if cmd := a.navigateBack(); cmd != nil {
		return cmd
	}
	item, ok := a.sidebar.SelectedItem()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		return ChannelSelectedMsg{ID: item.ID, Name: item.Name, Type: item.Type}
	}
}

// leaveFailed says that Slack refused a leave or a close; the row stays.
func (a *App) leaveFailed(channelID, text string, err error) tea.Cmd {
	if a.leaving == channelID {
		a.leaving = ""
	}
	debuglog.General("leaving conversation %s: %v", channelID, err)
	return toastWithClear(a, text, 3*time.Second)
}
