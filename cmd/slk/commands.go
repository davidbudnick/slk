package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ids"
	slackclient "github.com/gammons/slk/internal/slack"
)

// commandRunner is the part of *slackclient.Client running a slash
// command needs.
type commandRunner interface {
	RunCommand(ctx context.Context, req slackclient.CommandRequest) error
	TeamID() string
}

// runCommand runs a slash command through client and translates Slack's
// refusal into core.CommandError, so the UI never sees slackclient types.
func runCommand(ctx context.Context, client commandRunner, known []core.SlashCommand, channelID ids.ChannelID,
	threadTS ids.ThreadTS, command, args string) error {
	err := client.RunCommand(ctx, slackclient.CommandRequest{
		ChannelID: string(channelID),
		ThreadTS:  string(threadTS),
		Command:   command,
		Args:      args,
		TeamID:    commandTeam(known, command, client.TeamID()),
	})
	var rej *slackclient.CommandRejectedError
	if errors.As(err, &rej) {
		return &core.CommandError{Code: rej.Code}
	}
	return err
}

// commandTeam is the team_id a command runs with, as the web client
// chooses it: the team the workspace's list gives the command, and
// none for a command the list does not have. On Enterprise Grid the
// list names the workspace while session is the organization, which
// chat.command refuses with invalid_arguments. Before the list has
// loaded, session is all there is to send.
func commandTeam(known []core.SlashCommand, command, session string) string {
	if known == nil {
		return session
	}
	for _, c := range known {
		if strings.EqualFold(c.Name, command) {
			if c.TeamID != "" {
				return c.TeamID
			}
			return session
		}
	}
	return ""
}

// appCommandLister is the part of *slackclient.Client the "/" menu's
// list needs.
type appCommandLister interface {
	AppCommands(ctx context.Context) ([]core.SlashCommand, error)
}

// loadAppCommands fetches the workspace's slash commands once and
// publishes them. A failure leaves the list unset.
func loadAppCommands(ctx context.Context, wctx *WorkspaceContext, client appCommandLister) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	list, err := client.AppCommands(ctx)
	if err != nil {
		debuglog.General("client.appCommands for %s: %v (the / menu stays closed)", wctx.TeamID, err)
		return
	}
	debuglog.General("client.appCommands for %s: %d commands", wctx.TeamID, len(list))
	wctx.SetAppCommands(list)
}
