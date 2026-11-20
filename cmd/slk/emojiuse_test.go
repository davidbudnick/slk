package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
)

type fakeEmojiUsage struct {
	usage slackclient.EmojiUsage
	err   error
}

func (f fakeEmojiUsage) EmojiUsage(context.Context) (slackclient.EmojiUsage, error) {
	return f.usage, f.err
}

// Slack's counts decide the order, as in its own pickers; uses made
// from slk, which Slack does not count, are added on. Before Slack's
// counts are read, slk's own order stands.
func TestRankEmoji(t *testing.T) {
	slack := map[string]int{"pepe-salute": 102, "pepe_ez": 90, "pepe": 81, "ez": 2, "tada": 0}
	local := map[string]int{"ez": 3, "rocket": 1, "pepe": 9}
	localOrder := []string{"rocket", "ez", "pepe"}

	got := rankEmoji(slack, local, localOrder, 0)
	if want := []string{"pepe-salute", "pepe", "pepe_ez", "ez", "rocket"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rankEmoji = %v; want %v (pepe passes pepe_ez with slk's 9 uses, tada was never used)", got, want)
	}
	if got := rankEmoji(slack, local, localOrder, 2); !reflect.DeepEqual(got, []string{"pepe-salute", "pepe"}) {
		t.Errorf("capped at 2 = %v", got)
	}
	if got := rankEmoji(map[string]int{"b": 5, "a": 5, "c": 5}, nil, nil, 0); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("a tie = %v; want it broken by name", got)
	}
	for name, noSlack := range map[string]map[string]int{"not read": nil, "empty": {}} {
		if got := rankEmoji(noSlack, local, localOrder, 0); !reflect.DeepEqual(got, localOrder) {
			t.Errorf("Slack's counts %s: rankEmoji = %v; want slk's own order %v", name, got, localOrder)
		}
	}
	if got := rankEmoji(nil, local, localOrder, 1); !reflect.DeepEqual(got, []string{"rocket"}) {
		t.Errorf("slk's own order capped at 1 = %v", got)
	}
}

// The frequently used are the most picked in Slack's frecency store,
// the more recent first among equals, under the names slk lists emoji
// by: a standard emoji's code points become its short name.
func TestFrequentEmoji(t *testing.T) {
	recent := []slackclient.EmojiFrecency{
		{ID: "pepe-king", Count: 6, LastUsed: 50},
		{ID: "pepe-delux", Count: 5, LastUsed: 10},
		{ID: "newer-five", Count: 5, LastUsed: 90},
		{ID: "1f44c", Count: 701, LastUsed: 70},
		{ID: "1f44d-1f3fd", Count: 40, LastUsed: 60},
		{ID: "2764-fe0f", Count: 30, LastUsed: 60},
		{ID: "pepe_ez", Count: 46, LastUsed: 80},
	}
	got := frequentEmoji(recent, 6)
	want := map[string]bool{"ok_hand": true, "pepe_ez": true, "+1": true, "heart": true, "pepe-king": true, "newer-five": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("frequentEmoji = %v; want %v (pepe-delux loses the tie to the more recent)", got, want)
	}
	if got := frequentEmoji(nil, 40); len(got) != 0 {
		t.Errorf("no records: %v", got)
	}
	for id, want := range map[string]string{
		"1f44c": "ok_hand", "1f44d": "+1", "1f44d-1f3fb": "+1", "2764-fe0f": "heart", "2764": "heart",
		"pepe_ez": "pepe_ez", "100": "100", "dead-beef": "dead-beef", "fffff": "fffff",
	} {
		if got := frecencyEmojiName(id); got != want {
			t.Errorf("frecencyEmojiName(%q) = %q; want %q", id, got, want)
		}
	}
}

func TestLoadEmojiUse(t *testing.T) {
	wctx := &WorkspaceContext{TeamID: "T1"}
	sender := &captureSender{}
	if use := wctx.EmojiUse(); use.counts != nil || use.frequent != nil {
		t.Fatal("a workspace starts with emoji use")
	}

	loadEmojiUse(context.Background(), wctx, fakeEmojiUsage{err: errors.New("not_authed")}, sender)
	if wctx.EmojiUse().counts != nil || len(sender.sent) != 0 {
		t.Errorf("a failed read published counts or told the UI: %v, %v", wctx.EmojiUse(), sender.sent)
	}

	loadEmojiUse(context.Background(), wctx, fakeEmojiUsage{usage: slackclient.EmojiUsage{
		Counts: map[string]int{"tada": 3, "pepe_ez": 90},
		Recent: []slackclient.EmojiFrecency{{ID: "pepe_ez", Count: 46, LastUsed: 5}},
	}}, sender)
	use := wctx.EmojiUse()
	if use.counts["tada"] != 3 || !use.frequent["pepe_ez"] || use.frequent["tada"] {
		t.Errorf("EmojiUse = %+v; want the counts, and pepe_ez alone frequent", use)
	}
	if len(sender.sent) != 1 || sender.sent[0] != (ui.EmojiUseLoadedMsg{TeamID: "T1"}) {
		t.Errorf("UI messages = %v; want one EmojiUseLoadedMsg for T1", sender.sent)
	}

	var none *WorkspaceContext
	if none.EmojiUse().counts != nil {
		t.Error("no workspace has emoji use")
	}
}
