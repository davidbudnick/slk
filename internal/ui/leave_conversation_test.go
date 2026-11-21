package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// leaveApp is slashApp over a sidebar of general (C1), random (C2), the
// private secret (G1) and a DM (D1), with the composer in on. What is
// sent to ChannelService.Leave and Close is collected in left and
// closed, and answered with fail when that is set.
func leaveApp(t *testing.T, on string, fail error) (a *App, left, closed *[]string) {
	t.Helper()
	a, _ = slashApp(t, nil)
	a.SetChannels([]sidebar.ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
		{ID: "C2", Name: "random", Type: "channel"},
		{ID: "G1", Name: "secret", Type: "private"},
		{ID: "D1", Name: "Ann Lee", Type: "dm", DMUserID: "U1"},
	})
	a.channelFinder.SetItems([]core.ChannelFinderItem{
		{ID: "C1", Name: "general", Type: "channel", Joined: true},
		{ID: "C2", Name: "random", Type: "channel", Joined: true},
		{ID: "G1", Name: "secret", Type: "private", Joined: true},
		{ID: "D1", Name: "Ann Lee", Type: "dm", Joined: true},
	})
	a.activeChannelID = on
	a.compose.SetDraftContext("T1", on, "")
	a.sidebar.SelectByID("C2")
	left, closed = &[]string{}, &[]string{}
	a.SetChannelService(core.NewChannelService(core.ChannelServiceFuncs{
		Leave: func(id ids.ChannelID, name string) core.Msg {
			*left = append(*left, string(id))
			if fail != nil {
				return ChannelLeaveFailedMsg{ChannelID: string(id), Name: name, Err: fail}
			}
			return ChannelLeftMsg{TeamID: "T1", ChannelID: string(id)}
		},
		Close: func(id ids.ChannelID, name string) core.Msg {
			*closed = append(*closed, string(id))
			return ConversationClosedMsg{TeamID: "T1", ChannelID: string(id)}
		},
	}))
	return a, left, closed
}

// finderItem returns the finder's row for a conversation.
func finderItem(a *App, id string) (core.ChannelFinderItem, bool) {
	for _, it := range a.channelFinder.Items() {
		if it.ID == id {
			return it, true
		}
	}
	return core.ChannelFinderItem{}, false
}

// reduceFirst reduces the first T cmd delivers and returns the command
// that reduction asks for.
func reduceFirst[T any](t *testing.T, a *App, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	_, next := a.Update(deliveredMsgs[T](t, cmd)[0])
	return next
}

// /leave, and its aliases, leave the channel they are typed in: the row
// goes when Slack answers, the finder offers the channel as one to join,
// and the view moves to another conversation.
func TestSlashLeave_LeavesTheChannelAndMovesOff(t *testing.T) {
	for _, name := range []string{"/leave", "/close", "/part"} {
		t.Run(name, func(t *testing.T) {
			a, left, closed := leaveApp(t, "C1", nil)
			a.compose.SetValue(name)
			move := reduceFirst[ChannelLeftMsg](t, a, pressEnter(a))

			if !slices.Equal(*left, []string{"C1"}) || len(*closed) != 0 {
				t.Fatalf("Leave calls = %v, Close calls = %v; want C1 left", *left, *closed)
			}
			if sidebarHas(a, "C1") || !sidebarHas(a, "C2") {
				t.Errorf("sidebar lists %+v; want general alone gone", a.sidebar.Items())
			}
			if it, ok := finderItem(a, "C1"); !ok || it.Joined {
				t.Errorf("finder row = %+v (found %v); want general kept as one to join", it, ok)
			}
			if to := deliveredMsgs[ChannelSelectedMsg](t, move); to[0].ID != "C2" {
				t.Errorf("the view moved to %q; want random", to[0].ID)
			}
			if a.leaving != "" {
				t.Errorf("still leaving %q after Slack answered", a.leaving)
			}
			if _, again := a.Update(ChannelLeftMsg{TeamID: "T1", ChannelID: "C1"}); again != nil {
				t.Error("Slack's own event for the same leave moved the view a second time")
			}
		})
	}
}

// A private channel asks first, and only a yes leaves it; the finder
// then stops offering it.
func TestSlashLeave_PrivateChannelAsksFirst(t *testing.T) {
	a, left, _ := leaveApp(t, "G1", nil)
	a.compose.SetValue("/leave")
	_ = pressEnter(a)
	if a.mode != ModeConfirm || !dialogShows(a, "Leave secret?") {
		t.Fatalf("mode = %v; want the prompt naming the channel", a.mode)
	}
	if _, cmd := a.Update(keyPress('n')); cmd != nil || len(*left) != 0 || a.leaving != "" {
		t.Fatalf("a no left the channel: calls %v, leaving %q", *left, a.leaving)
	}

	a.SetMode(ModeInsert)
	a.compose.SetValue("/leave")
	_ = pressEnter(a)
	_, yes := a.Update(keyPress('y'))
	_ = reduceFirst[ChannelLeftMsg](t, a, reduceFirst[leaveChannelMsg](t, a, yes))
	if !slices.Equal(*left, []string{"G1"}) || sidebarHas(a, "G1") {
		t.Errorf("Leave calls = %v, sidebar %+v; want secret left and gone", *left, a.sidebar.Items())
	}
	if _, ok := finderItem(a, "G1"); ok {
		t.Error("the finder still offers a private channel the user left")
	}
}

// In a DM /leave closes it, as the "x" on its row does, and moves off it.
func TestSlashLeave_ClosesADM(t *testing.T) {
	a, left, closed := leaveApp(t, "D1", nil)
	a.compose.SetValue("/leave")
	move := reduceFirst[ConversationClosedMsg](t, a, pressEnter(a))
	if !slices.Equal(*closed, []string{"D1"}) || len(*left) != 0 {
		t.Fatalf("Close calls = %v, Leave calls = %v; want D1 closed", *closed, *left)
	}
	if sidebarHas(a, "D1") {
		t.Error("the closed DM's row stayed")
	}
	if to := deliveredMsgs[ChannelSelectedMsg](t, move); to[0].ID != "C2" {
		t.Errorf("the view moved to %q; want random", to[0].ID)
	}
	if it, ok := finderItem(a, "D1"); !ok || !it.Joined {
		t.Errorf("finder row = %+v (found %v); want the DM kept, to open it again", it, ok)
	}
}

// /leave takes no arguments, and a leave Slack refuses keeps the row and says so.
func TestSlashLeave_Refusals(t *testing.T) {
	a, left, _ := leaveApp(t, "C1", nil)
	a.compose.SetValue("/leave #random")
	_ = pressEnter(a)
	if len(*left) != 0 || a.compose.Value() != "/leave #random" || !strings.Contains(statusbarText(a), "Usage: /leave") {
		t.Errorf("calls %v, compose %q, status bar %q; want the usage line and the text back", *left, a.compose.Value(), statusbarText(a))
	}

	a, left, _ = leaveApp(t, "C1", errors.New("cant_leave_general"))
	a.compose.SetValue("/leave")
	_ = reduceFirst[ChannelLeaveFailedMsg](t, a, pressEnter(a))
	if !slices.Equal(*left, []string{"C1"}) || !sidebarHas(a, "C1") || a.activeChannelID != "C1" || a.leaving != "" {
		t.Errorf("calls %v, on %q, leaving %q; want the row kept and the view left alone", *left, a.activeChannelID, a.leaving)
	}
	if !strings.Contains(statusbarText(a), "Could not leave general") {
		t.Errorf("status bar = %q, want the refusal", statusbarText(a))
	}
}

// A leave made on another client takes the row out and keeps the view;
// one for another workspace, or of a DM, changes nothing.
func TestChannelLeft_OnAnotherClient(t *testing.T) {
	a, _, _ := leaveApp(t, "C1", nil)
	a.Update(ChannelLeftMsg{TeamID: "T2", ChannelID: "C2"})
	a.Update(ChannelLeftMsg{TeamID: "T1", ChannelID: "D1"})
	if !sidebarHas(a, "C2") || !sidebarHas(a, "D1") {
		t.Fatalf("sidebar lost a row it should keep: %+v", a.sidebar.Items())
	}
	_, cmd := a.Update(ChannelLeftMsg{TeamID: "T1", ChannelID: "C1"})
	if sidebarHas(a, "C1") {
		t.Error("the left channel's row stayed")
	}
	if cmd != nil || a.activeChannelID != "C1" {
		t.Errorf("the view moved off a channel left elsewhere (on %q)", a.activeChannelID)
	}
}
