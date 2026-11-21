package main

import (
	"context"
	"errors"
	"testing"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/ui/sidebar"
	"github.com/slack-go/slack"
)

// closeHandler is a handler over a workspace with a channel, a DM and
// a group DM, whose UI messages land in the returned sender.
func closeHandler(active bool) (*rtmEventHandler, *captureSender) {
	sender := &captureSender{}
	wctx := &WorkspaceContext{
		UserID:            "UME",
		UserHandle:        "me",
		UserNames:         newUserNameStore(nil),
		UserNamesByHandle: map[string]string{"ann": "Ann Lee", "bo": "Bo Ito"},
		Channels: []sidebar.ChannelItem{
			{ID: "C1", Name: "general", Type: "channel"},
			{ID: "D1", Name: "Ann Lee", Type: "dm", DMUserID: "UANN"},
			{ID: "G1", Name: "Ann Lee, Bo Ito", Type: "group_dm"},
		},
		FinderItems: []core.ChannelFinderItem{{ID: "D1", Name: "Ann Lee", Type: "dm", Joined: true}},
	}
	return &rtmEventHandler{
		wsCtx:        wctx,
		workspaceID:  "T1",
		cfg:          config.Config{},
		program:      sender,
		isActive:     func() bool { return active },
		channelNames: map[string]string{"D1": "Ann Lee"},
		channelTypes: map[string]string{"D1": "dm"},
	}, sender
}

// A close on any of the user's clients takes the row out of this
// workspace's sidebar set and tells the UI. The finder keeps the
// conversation, which is how it is opened again.
func TestOnConversationClosed_RemovesTheRowAndTellsTheUI(t *testing.T) {
	h, sender := closeHandler(true)

	h.OnConversationClosed("D1")

	for _, item := range h.wsCtx.Channels {
		if item.ID == "D1" {
			t.Fatalf("the closed DM is still in the sidebar set: %+v", h.wsCtx.Channels)
		}
	}
	if len(h.wsCtx.Channels) != 2 {
		t.Errorf("sidebar set = %+v; want the channel and the group DM left", h.wsCtx.Channels)
	}
	if len(h.wsCtx.FinderItems) != 1 {
		t.Errorf("finder items = %+v; want the closed DM kept", h.wsCtx.FinderItems)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("UI messages = %+v; want one", sender.sent)
	}
	if got, ok := sender.sent[0].(ui.ConversationClosedMsg); !ok || got.TeamID != "T1" || got.ChannelID != "D1" {
		t.Errorf("UI message = %#v; want ConversationClosedMsg for T1/D1", sender.sent[0])
	}

	// Slack echoes a close made here; the second pass finds no row and
	// says nothing.
	h.OnConversationClosed("D1")
	if len(sender.sent) != 1 {
		t.Errorf("a repeated close sent %d messages; want still 1", len(sender.sent))
	}
}

func TestOnConversationClosed_LeavesWhatItShould(t *testing.T) {
	h, sender := closeHandler(true)
	h.OnConversationClosed("C1")
	h.OnConversationClosed("D404")
	if len(h.wsCtx.Channels) != 3 || len(sender.sent) != 0 {
		t.Errorf("a channel or an unknown conversation was closed: rows %+v, messages %+v", h.wsCtx.Channels, sender.sent)
	}

	// An inactive workspace's set changes; its UI is told on the switch.
	h, sender = closeHandler(false)
	h.OnConversationClosed("G1")
	if len(h.wsCtx.Channels) != 2 || len(sender.sent) != 0 {
		t.Errorf("inactive workspace: rows %+v, messages %+v; want the row gone and no message", h.wsCtx.Channels, sender.sent)
	}
}

// Slack names a group DM that is opened again by its ID alone. The row
// is built from what conversations.info returns, and a lookup that
// fails adds no nameless row.
func TestOnConversationOpened_LooksUpAGroupDMNamedByIDAlone(t *testing.T) {
	reopened := slack.Channel{GroupConversation: slack.GroupConversation{
		Conversation: slack.Conversation{ID: "G1", IsMpIM: true},
	}}

	h, sender := closeHandler(true)
	h.OnConversationClosed("G1")
	var asked string
	h.resolveConversation = func(_ context.Context, id string) (*slack.Channel, error) {
		asked = id
		full := reopened
		full.Name = "mpdm-me--ann--bo-1"
		return &full, nil
	}
	h.OnConversationOpened(reopened)

	if asked != "G1" {
		t.Fatalf("conversations.info asked for %q; want G1", asked)
	}
	var row sidebar.ChannelItem
	for _, item := range h.wsCtx.Channels {
		if item.ID == "G1" {
			row = item
		}
	}
	if row.Name != "Ann Lee, Bo Ito" || row.Type != "group_dm" || row.GroupSize != 2 {
		t.Errorf("reopened row = %+v; want Ann Lee, Bo Ito as a group DM of 2", row)
	}
	if got, ok := sender.sent[len(sender.sent)-1].(ui.ConversationOpenedMsg); !ok || got.Item.ID != "G1" {
		t.Errorf("last UI message = %#v; want ConversationOpenedMsg for G1", sender.sent[len(sender.sent)-1])
	}

	h, _ = closeHandler(true)
	h.OnConversationClosed("G1")
	h.resolveConversation = func(context.Context, string) (*slack.Channel, error) {
		return nil, errors.New("channel_not_found")
	}
	h.OnConversationOpened(reopened)
	if len(h.wsCtx.Channels) != 2 {
		t.Errorf("a failed lookup still added a row: %+v", h.wsCtx.Channels)
	}
}
