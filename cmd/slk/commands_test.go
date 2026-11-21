package main

import (
	"context"
	"errors"
	"testing"

	"github.com/gammons/slk/internal/core"
	slackclient "github.com/gammons/slk/internal/slack"
)

type fakeCommandRunner struct {
	got  slackclient.CommandRequest
	err  error
	team string
}

func (f *fakeCommandRunner) RunCommand(_ context.Context, req slackclient.CommandRequest) error {
	f.got = req
	return f.err
}

func (f *fakeCommandRunner) TeamID() string { return f.team }

func TestRunCommandMapsFields(t *testing.T) {
	f := &fakeCommandRunner{team: "T0EXAMPLE01"}
	if err := runCommand(context.Background(), f, nil, "C0EXAMPLE01", "10.0", "/giphy", "cats"); err != nil {
		t.Fatal(err)
	}
	want := slackclient.CommandRequest{ChannelID: "C0EXAMPLE01", ThreadTS: "10.0", Command: "/giphy", Args: "cats", TeamID: "T0EXAMPLE01"}
	if f.got != want {
		t.Errorf("request = %+v, want %+v", f.got, want)
	}
}

// The team a command runs with is the one the workspace's list gives
// it. On Enterprise Grid that is the workspace, where the session's own
// team is the organization: sending the organization made chat.command
// refuse every command with invalid_arguments. A command the list does
// not have goes without a team, as the web client sends it.
func TestRunCommandSendsTheCommandsOwnTeam(t *testing.T) {
	known := []core.SlashCommand{
		{Name: "/giphy", TeamID: "T0WORKSPACE"},
		{Name: "/noteam"},
	}
	tests := []struct {
		name    string
		known   []core.SlashCommand
		command string
		want    string
	}{
		{"listed: the list's team", known, "/giphy", "T0WORKSPACE"},
		{"listed, any case", known, "/GIPHY", "T0WORKSPACE"},
		{"listed without a team: the session's", known, "/noteam", "E0ORG"},
		{"not listed: none", known, "/notarealcommand", ""},
		{"list loaded but empty: none", []core.SlashCommand{}, "/giphy", ""},
		{"list not loaded: the session's", nil, "/giphy", "E0ORG"},
	}
	for _, tt := range tests {
		f := &fakeCommandRunner{team: "E0ORG"}
		if err := runCommand(context.Background(), f, tt.known, "C1", "", tt.command, ""); err != nil {
			t.Fatal(err)
		}
		if f.got.TeamID != tt.want {
			t.Errorf("%s: team_id = %q; want %q", tt.name, f.got.TeamID, tt.want)
		}
	}
}

// loadAppCommands publishes what client.appCommands returns, and leaves
// the list unset when the call fails, so commands still run as before.
func TestLoadAppCommands(t *testing.T) {
	wctx := &WorkspaceContext{TeamID: "T1"}
	if wctx.AppCommands() != nil {
		t.Fatal("a workspace starts with a command list")
	}
	loadAppCommands(context.Background(), wctx, fakeCommandLister{err: errors.New("enterprise_is_restricted")})
	if wctx.AppCommands() != nil {
		t.Error("a failed fetch published a list")
	}
	loadAppCommands(context.Background(), wctx, fakeCommandLister{list: []core.SlashCommand{{Name: "/giphy"}}})
	if got := wctx.AppCommands(); len(got) != 1 || got[0].Name != "/giphy" {
		t.Errorf("AppCommands = %+v; want /giphy", got)
	}
	var none *WorkspaceContext
	if none.AppCommands() != nil {
		t.Error("no workspace has commands")
	}
}

type fakeCommandLister struct {
	list []core.SlashCommand
	err  error
}

func (f fakeCommandLister) AppCommands(context.Context) ([]core.SlashCommand, error) {
	return f.list, f.err
}

func TestRunCommandTranslatesRejection(t *testing.T) {
	f := &fakeCommandRunner{err: &slackclient.CommandRejectedError{Code: "unknown_command"}}
	err := runCommand(context.Background(), f, nil, "C1", "", "/nope", "")
	var ce *core.CommandError
	if !errors.As(err, &ce) || ce.Code != "unknown_command" {
		t.Fatalf("err = %v, want *core.CommandError{unknown_command}", err)
	}
}

func TestRunCommandPassesOtherErrorsThrough(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeCommandRunner{err: boom}
	if err := runCommand(context.Background(), f, nil, "C1", "", "/giphy", ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}
