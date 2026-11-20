package main

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gammons/slk/internal/debuglog"
	emojiwidth "github.com/gammons/slk/internal/emoji"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
)

// frequentEmojiCount is how many emoji Slack treats as frequently
// used: the most picked in its frecency store. In the data this was
// worked out from, the 39th most picked led Slack's autocomplete and
// the 41st did not.
const frequentEmojiCount = 40

// emojiUse is what slk ranks emoji by once Slack's own records have
// been read: counts is Slack's all-time use count by short name, and
// frequent the names Slack treats as frequently used.
type emojiUse struct {
	counts   map[string]int
	frequent map[string]bool
}

// emojiUsageReader is the part of *slackclient.Client the emoji
// ranking needs.
type emojiUsageReader interface {
	EmojiUsage(ctx context.Context) (slackclient.EmojiUsage, error)
}

// loadEmojiUse reads the user's emoji use from Slack once and publishes
// it, then tells the UI so the pickers rank by it. A failure leaves it
// unset, and the pickers rank by slk's own history as before.
func loadEmojiUse(ctx context.Context, wctx *WorkspaceContext, client emojiUsageReader, send teaSender) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	usage, err := client.EmojiUsage(ctx)
	if err != nil {
		debuglog.General("emoji use for %s: %v (ranking emoji by slk's own history)", wctx.TeamID, err)
		return
	}
	use := emojiUse{counts: usage.Counts, frequent: frequentEmoji(usage.Recent, frequentEmojiCount)}
	debuglog.General("emoji use for %s: %d counted, %d in the frecency store, %d frequent",
		wctx.TeamID, len(use.counts), len(usage.Recent), len(use.frequent))
	wctx.SetEmojiUse(use)
	if send != nil {
		send.Send(ui.EmojiUseLoadedMsg{TeamID: wctx.TeamID})
	}
}

// frequentEmoji returns the short names of the n most picked emoji in
// Slack's frecency store, the more recently picked first among equals.
func frequentEmoji(recent []slackclient.EmojiFrecency, n int) map[string]bool {
	ranked := append([]slackclient.EmojiFrecency(nil), recent...)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Count != ranked[j].Count {
			return ranked[i].Count > ranked[j].Count
		}
		return ranked[i].LastUsed > ranked[j].LastUsed
	})
	if len(ranked) > n {
		ranked = ranked[:n]
	}
	frequent := make(map[string]bool, len(ranked))
	for _, r := range ranked {
		if name := frecencyEmojiName(r.ID); name != "" {
			frequent[name] = true
		}
	}
	return frequent
}

// standardEmojiNames maps a standard emoji's code points, variation
// selectors and skin tones left out, to Slack's canonical short name.
var standardEmojiNames = sync.OnceValue(func() map[string]string {
	names := map[string]string{}
	for code, glyph := range emojiwidth.CodeMap() {
		name := strings.Trim(code, ":")
		key := emojiKey([]rune(glyph))
		if key == "" || emojiwidth.CanonicalSlackName(name, nil) != name {
			continue
		}
		if old, dup := names[key]; !dup || name < old {
			names[key] = name
		}
	}
	return names
})

// emojiKey spells code points as the frecency store does, in hex joined
// by "-", without variation selectors and skin tone modifiers.
func emojiKey(points []rune) string {
	parts := make([]string, 0, len(points))
	for _, r := range points {
		if r == 0xfe0f || (r >= 0x1f3fb && r <= 0x1f3ff) {
			continue
		}
		parts = append(parts, strconv.FormatInt(int64(r), 16))
	}
	return strings.Join(parts, "-")
}

// frecencyEmojiName turns a frecency store ID into the emoji's short
// name: a standard emoji is keyed by its code points, anything else by
// its name already.
func frecencyEmojiName(id string) string {
	var points []rune
	for _, part := range strings.Split(id, "-") {
		n, err := strconv.ParseInt(part, 16, 32)
		if err != nil || len(part) < 4 {
			return id
		}
		points = append(points, rune(n))
	}
	if name, ok := standardEmojiNames()[emojiKey(points)]; ok {
		return name
	}
	return id
}

// rankEmoji orders the emoji the user has used, most used first, as
// Slack's pickers do: by Slack's own count plus the uses made from slk,
// which Slack does not count. Ties go by name. Without Slack's counts
// the order is localOrder, slk's own frecency. limit caps the result;
// 0 or less means no cap.
func rankEmoji(slack, local map[string]int, localOrder []string, limit int) []string {
	names := localOrder
	if len(slack) > 0 {
		total := make(map[string]int, len(slack)+len(local))
		for name, n := range slack {
			total[name] += n
		}
		for name, n := range local {
			total[name] += n
		}
		names = make([]string, 0, len(total))
		for name, n := range total {
			if n > 0 {
				names = append(names, name)
			}
		}
		sort.Slice(names, func(i, j int) bool {
			if a, b := total[names[i]], total[names[j]]; a != b {
				return a > b
			}
			return names[i] < names[j]
		})
	}
	if limit > 0 && len(names) > limit {
		names = names[:limit]
	}
	return names
}
