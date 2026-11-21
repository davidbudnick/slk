package ui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

// emojiHistoryApp returns an App whose emoji history is kept in
// memory, most recent first, and the history itself.
func emojiHistoryApp(t *testing.T, opts ...testOpt) (*App, *[]string) {
	t.Helper()
	a := newTestApp(t, opts...)
	var history []string
	a.SetReactionService(core.NewReactionService(
		func(ids.ChannelID, ids.MessageTS, string) error { return nil },
		func(ids.ChannelID, ids.MessageTS, string) error { return nil },
		func(limit int) []core.EmojiEntry {
			var out []core.EmojiEntry
			for _, n := range history {
				if len(out) == limit {
					break
				}
				out = append(out, core.EmojiEntry{Name: n})
			}
			return out
		},
		func(name string) {
			history = append([]string{name}, slices.DeleteFunc(history, func(n string) bool { return n == name })...)
		},
	))
	return a, &history
}

// autocompleteRows is what the channel composer's emoji autocomplete
// shows after typing s, one name per row in order.
func autocompleteRows(t *testing.T, a *App, s string) []string {
	t.Helper()
	a.SetMode(ModeInsert)
	_ = a.compose.Focus()
	typeInCompose(a, s)
	view := stripANSI(a.compose.EmojiPickerView(80))
	if view == "" {
		t.Fatalf("no emoji autocomplete after typing %q", s)
	}
	var rows []string
	for _, line := range strings.Split(view, "\n") {
		fields := strings.Fields(strings.Trim(line, "│╭╮╰╯─▌ "))
		if len(fields) > 0 {
			rows = append(rows, strings.Trim(fields[len(fields)-1], ":"))
		}
	}
	return rows
}

func TestSentMessageEmojiLeadTheAutocomplete(t *testing.T) {
	a, history := emojiHistoryApp(t, withActiveChannel("C1"))
	if rows := autocompleteRows(t, a, ":ro"); len(rows) == 0 || rows[0] == "rose" {
		t.Fatalf("precondition: rows = %v, want an alphabetical list not led by rose", rows)
	}
	a.compose.SetValue("")
	a.compose.CloseEmoji()

	a.Update(SendMessageMsg{ChannelID: "C1", Text: "for you :rose: :thumbsup: 12:30:45"})

	if want := []string{"+1", "rose"}; !reflect.DeepEqual(*history, want) {
		t.Errorf("history = %v, want %v (canonical names, no :30:)", *history, want)
	}
	if rows := autocompleteRows(t, a, ":ro"); len(rows) == 0 || rows[0] != "rose" {
		t.Errorf("autocomplete rows = %v, want rose first", rows)
	}
}

func TestSentThreadReplyEmojiAreRecorded(t *testing.T) {
	a, history := emojiHistoryApp(t, withActiveChannel("C1"))
	a.Update(SendThreadReplyMsg{ChannelID: "C1", ThreadTS: "1.0", Text: "done :tada:"})
	if want := []string{"tada"}; !reflect.DeepEqual(*history, want) {
		t.Errorf("history = %v, want %v", *history, want)
	}
}

func TestTogglingAPillOnRecordsTheEmoji(t *testing.T) {
	a, history := emojiHistoryApp(t, withActiveChannel("C1"))
	a.SetCurrentUserID("U1")
	msg := messages.MessageItem{TS: "1.0", Reactions: []messages.ReactionItem{
		{Emoji: "eyes", Count: 1, UserIDs: []string{"U2"}},
		{Emoji: "tada", Count: 1, UserIDs: []string{"U1"}, HasReacted: true},
	}}

	_ = a.toggleReactionOnMessageItem("C1", msg, "eyes")
	if want := []string{"eyes"}; !reflect.DeepEqual(*history, want) {
		t.Errorf("after adding eyes, history = %v, want %v", *history, want)
	}
	// Taking a reaction back is not a use.
	_ = a.toggleReactionOnMessageItem("C1", msg, "tada")
	if want := []string{"eyes"}; !reflect.DeepEqual(*history, want) {
		t.Errorf("after removing tada, history = %v, want %v", *history, want)
	}
}

// The autocomplete lists a much-used emoji whose name only contains
// what was typed, after the used ones that start with it and before
// everything unused: where Slack's ":ez" list put it.
func TestAutocompleteOrdersAsSlackDoes(t *testing.T) {
	a, history := emojiHistoryApp(t, withActiveTeam("T1"))
	a.SetCustomEmoji(map[string]string{
		"ez": "https://e/ez.png", "eze": "https://e/eze.png", "ez_llama": "https://e/l.png",
		"ez_robot": "https://e/r.png", "pepe_ez": "https://e/p.png",
	})
	*history = []string{"pepe_ez", "ez"}
	a.Update(EmojiUseLoadedMsg{TeamID: "T2"})
	if got := autocompleteRows(t, a, ":ez"); reflect.DeepEqual(got[:2], []string{"ez", "pepe_ez"}) {
		t.Fatalf("another workspace's counts changed this one's ranking: %v", got)
	}
	a.compose.Reset()

	a.Update(EmojiUseLoadedMsg{TeamID: "T1"})
	got := autocompleteRows(t, a, ":ez")
	if want := []string{"ez", "pepe_ez", "ez_llama", "ez_robot", "eze"}; !reflect.DeepEqual(got, want) {
		t.Errorf("autocomplete for :ez = %v; want %v", got, want)
	}
}

// The frequently used lead the autocomplete in the history's order,
// with the exactly named emoji among them; an emoji used more but not
// lately waits behind them.
func TestAutocompleteLeadsWithTheFrequentlyUsed(t *testing.T) {
	a := newTestApp(t, withActiveTeam("T1"))
	a.SetReactionService(core.NewReactionService(
		func(ids.ChannelID, ids.MessageTS, string) error { return nil },
		func(ids.ChannelID, ids.MessageTS, string) error { return nil },
		func(int) []core.EmojiEntry {
			return []core.EmojiEntry{
				{Name: "pepe-salute", Frequent: true},
				{Name: "pepe"},
				{Name: "pepesad"},
				{Name: "pepe_fist", Frequent: true},
			}
		},
		func(string) {},
	))
	a.SetCustomEmoji(map[string]string{
		"pepe": "https://e/1.png", "pepe-salute": "https://e/2.png", "pepesad": "https://e/3.png",
		"pepe_fist": "https://e/4.png", "pepe-4560": "https://e/5.png",
	})
	got := autocompleteRows(t, a, ":pepe")
	if want := []string{"pepe-salute", "pepe", "pepe_fist", "pepesad", "pepe-4560"}; !reflect.DeepEqual(got, want) {
		t.Errorf("autocomplete for :pepe = %v; want %v", got, want)
	}
}
