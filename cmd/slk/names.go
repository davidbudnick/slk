package main

import (
	"sort"
	"strings"
	"sync/atomic"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/slack/boot"
	"github.com/gammons/slk/internal/slackfmt"
	"github.com/slack-go/slack"
)

// fullNames is [appearance] full_names, set once at startup.
var fullNames atomic.Bool

// shownName returns a person's full name when full names are on and
// Slack has one, else name.
func shownName(realName, name string) string {
	if fullNames.Load() && realName != "" {
		return realName
	}
	return name
}

// cachedUserName returns the name to show for a cached user.
func cachedUserName(u cache.User) string {
	name := u.DisplayName
	if name == "" {
		name = u.Name
	}
	return shownName(u.RealName, name)
}

// groupName names a group DM by its other members, in the alphabetical
// order Slack lists them, when their IDs are known and each has a
// name; else by the handles in the conversation's own name.
func groupName(ch slack.Channel, wctx *WorkspaceContext) string {
	byHandle := slackfmt.FormatMPDMName(ch.Name, wctx.UserHandle, func(h string) string {
		return wctx.UserNamesByHandle[h]
	})
	if byHandle == ch.Name {
		return byHandle
	}
	names := make([]string, 0, len(ch.Members))
	for _, id := range ch.Members {
		if id == wctx.UserID {
			continue
		}
		name, ok := wctx.UserNames.Get(id)
		if !ok || name == "" {
			return byHandle
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return byHandle
	}
	sort.SliceStable(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	return strings.Join(names, ", ")
}

// resolveGroupMembers fetches, in one batch, the group DM members slk
// has no name for, or no full name when those are shown, so that
// groupName can list every member. It blocks on the network.
func resolveGroupMembers(wctx *WorkspaceContext, db *cache.DB, channels []slack.Channel) {
	seen := map[string]bool{wctx.UserID: true}
	var ids []string
	for _, ch := range channels {
		if !ch.IsMpIM {
			continue
		}
		for _, id := range ch.Members {
			if seen[id] {
				continue
			}
			seen[id] = true
			u, err := db.GetUser(id)
			if err == nil && cachedUserName(u) != "" && (!fullNames.Load() || u.RealName != "" || u.IsBot) {
				continue
			}
			ids = append(ids, id)
		}
	}
	wctx.UserResolver.ResolveNow(ids)
}

// reseedPeerNames re-reads the names of the DM peers, whose rows boot
// revalidation rewrote after the cache seed read them.
func reseedPeerNames(wctx *WorkspaceContext, db *cache.DB, ims []boot.IM) {
	for _, im := range ims {
		u, err := db.GetUser(im.UserID)
		if err != nil {
			continue
		}
		name := cachedUserName(u)
		if name == "" {
			continue
		}
		wctx.UserNames.Set(u.ID, name)
		if u.Name != "" {
			wctx.UserNamesByHandle[u.Name] = name
		}
	}
}
