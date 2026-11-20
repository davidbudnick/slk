package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gammons/slk/internal/core"
)

// ThreadSummary is defined in internal/core, which the TUI shares.
type ThreadSummary = core.ThreadSummary

// ListSubscribedThreads returns the workspace's subscribed-threads
// list — the authoritative set from thread_subscriptions joined
// against cached message/channel data for display. Replaces the v1
// "involved threads" heuristic with Slack-side subscription state.
//
// Threads with no cached messages still appear; their parent
// text/user fall back to "" and LastReplyTS falls back to the
// subscription's LastRead so sort still produces a sensible order.
//
// Ordering: newest LastReplyTS first.
//
// Unread is computed from the subscription's per-thread LastRead
// (not the per-channel channels.last_read_ts): unread iff the newest
// activity is later than LastRead. "Newest activity" is the max of the
// authoritative latest_reply watermark (from subscriptions.thread.getView)
// and the newest locally-cached reply — so a subscribed thread with
// unread replies renders unread even when those replies have not been
// fetched into the cache yet (the boot case). The only suppression is
// the optimistic just-sent window: a locally-cached newest reply known
// to be authored by self does not count as unread.
//
// ReplyCount is the larger of the parent's reply_count (Slack's own
// number) and the replies cached locally, so a thread whose replies were
// never fetched still reports its size. Replies carries the newest
// cached replies and Participants the parent's author, Slack's
// reply_users and the authors of those replies.
func (db *DB) ListSubscribedThreads(workspaceID, selfUserID string) ([]ThreadSummary, error) {
	const q = `
SELECT
    s.channel_id,
    s.thread_ts,
    COALESCE(c.name, ''),
    COALESCE(c.type, ''),
    s.last_read,
    s.latest_reply,
    COALESCE((SELECT user_id FROM messages
              WHERE channel_id = s.channel_id AND ts = s.thread_ts AND is_deleted = 0), ''),
    COALESCE((SELECT text FROM messages
              WHERE channel_id = s.channel_id AND ts = s.thread_ts AND is_deleted = 0), ''),
    (SELECT COUNT(*) FROM messages
     WHERE channel_id = s.channel_id AND thread_ts = s.thread_ts
       AND ts != s.thread_ts AND is_deleted = 0)
        AS reply_count,
    COALESCE(
        (SELECT MAX(ts) FROM messages
         WHERE channel_id = s.channel_id AND thread_ts = s.thread_ts AND is_deleted = 0),
        ''
    ) AS cached_max_ts,
    COALESCE(
        (SELECT user_id FROM messages
         WHERE channel_id = s.channel_id AND thread_ts = s.thread_ts AND is_deleted = 0
         ORDER BY ts DESC LIMIT 1),
        ''
    ) AS last_reply_by,
    COALESCE((SELECT reply_count FROM messages
              WHERE channel_id = s.channel_id AND ts = s.thread_ts AND is_deleted = 0), 0),
    COALESCE((SELECT CASE WHEN json_valid(raw_json)
                          THEN json_extract(raw_json, '$.reply_users') END
              FROM messages
              WHERE channel_id = s.channel_id AND ts = s.thread_ts AND is_deleted = 0), '')
FROM thread_subscriptions s
LEFT JOIN channels c ON c.id = s.channel_id
WHERE s.workspace_id = ? AND s.active = 1
`
	rows, err := db.conn.Query(q, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing subscribed threads: %w", err)
	}
	defer rows.Close()

	var out []ThreadSummary
	var replyUsers []string
	for rows.Next() {
		var s ThreadSummary
		var lastRead, latestReply, cachedMaxTS, replyUsersJSON string
		var parentReplyCount int
		if err := rows.Scan(
			&s.ChannelID,
			&s.ThreadTS,
			&s.ChannelName,
			&s.ChannelType,
			&lastRead,
			&latestReply,
			&s.ParentUserID,
			&s.ParentText,
			&s.ReplyCount,
			&cachedMaxTS,
			&s.LastReplyBy,
			&parentReplyCount,
			&replyUsersJSON,
		); err != nil {
			return nil, fmt.Errorf("scanning subscribed thread row: %w", err)
		}
		s.ParentTS = s.ThreadTS
		s.ReplyCount = max(s.ReplyCount, parentReplyCount)
		replyUsers = append(replyUsers, replyUsersJSON)

		// Effective newest activity = the max of the authoritative
		// getView watermark (latest_reply) and the newest reply we've
		// cached locally, falling back to last_read so uncached/read
		// threads still sort sensibly. Comparing string ts is valid:
		// Slack ts are fixed-width "secs.micros".
		effLatest := lastRead
		if cachedMaxTS > effLatest {
			effLatest = cachedMaxTS
		}
		if latestReply > effLatest {
			effLatest = latestReply
		}
		s.LastReplyTS = effLatest

		// Unread when the newest activity is past our read watermark.
		// Suppress only when that newest activity is a locally-cached
		// reply we know was authored by self (the optimistic just-sent
		// window, before last_read catches up). When the newest activity
		// comes from the authoritative latest_reply (author unknown /
		// uncached), we trust the ts comparison — this is the boot case
		// the old cached-reply-only heuristic got wrong.
		unread := effLatest > lastRead
		if unread && cachedMaxTS == effLatest && s.LastReplyBy == selfUserID {
			unread = false
		}
		s.Unread = unread
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	previews, err := db.threadPreviewReplies(workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range out {
		s := &out[i]
		copy(s.Replies[:], previews[threadKey{s.ChannelID, s.ThreadTS}])
		s.Participants = threadParticipants(*s, replyUsers[i])
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LastReplyTS > out[j].LastReplyTS
	})
	return out, nil
}

// threadKey identifies a thread within a workspace.
type threadKey struct{ channelID, threadTS string }

// threadPreviewReplies returns, for each subscribed thread with cached
// replies, its newest core.ThreadPreviewReplies of them, oldest first.
func (db *DB) threadPreviewReplies(workspaceID string) (map[threadKey][]core.ThreadReply, error) {
	const q = `
SELECT channel_id, thread_ts, user_id, text, ts FROM (
    SELECT m.channel_id, m.thread_ts, m.user_id, m.text, m.ts,
           ROW_NUMBER() OVER (PARTITION BY m.channel_id, m.thread_ts ORDER BY m.ts DESC) AS rn
    FROM thread_subscriptions s
    JOIN messages m ON m.channel_id = s.channel_id AND m.thread_ts = s.thread_ts
    WHERE s.workspace_id = ? AND s.active = 1
      AND m.ts != m.thread_ts AND m.is_deleted = 0
) WHERE rn <= ?
ORDER BY channel_id, thread_ts, ts
`
	rows, err := db.conn.Query(q, workspaceID, core.ThreadPreviewReplies)
	if err != nil {
		return nil, fmt.Errorf("listing thread preview replies: %w", err)
	}
	defer rows.Close()

	out := map[threadKey][]core.ThreadReply{}
	for rows.Next() {
		var k threadKey
		var r core.ThreadReply
		if err := rows.Scan(&k.channelID, &k.threadTS, &r.UserID, &r.Text, &r.TS); err != nil {
			return nil, fmt.Errorf("scanning thread preview reply: %w", err)
		}
		out[k] = append(out[k], r)
	}
	return out, rows.Err()
}

// threadParticipants lists a thread's distinct authors: the parent's,
// then Slack's reply_users (a JSON array, "" when absent), then the
// authors of the previewed replies.
func threadParticipants(s ThreadSummary, replyUsersJSON string) [core.ThreadPreviewParticipants]string {
	var users []string
	if replyUsersJSON != "" {
		_ = json.Unmarshal([]byte(replyUsersJSON), &users)
	}
	ids := append([]string{s.ParentUserID}, users...)
	for _, r := range s.PreviewReplies() {
		ids = append(ids, r.UserID)
	}
	var out [core.ThreadPreviewParticipants]string
	n := 0
	for _, id := range ids {
		if id == "" || n == len(out) {
			continue
		}
		seen := false
		for _, have := range out[:n] {
			seen = seen || have == id
		}
		if !seen {
			out[n] = id
			n++
		}
	}
	return out
}

// ThreadInvolvesUser reports whether the given thread (identified by
// workspaceID, channelID, threadTS) has any cached message authored
// by selfUserID or containing the angle-bracketed mention "<@selfUserID>".
// Used by the reconnect backfill to filter which threads warrant a
// conversations.replies catch-up call.
func (db *DB) ThreadInvolvesUser(workspaceID, channelID, threadTS, selfUserID string) (bool, error) {
	mention := "%<@" + selfUserID + ">%"
	const q = `
SELECT 1 FROM messages
WHERE workspace_id = ? AND channel_id = ? AND thread_ts = ?
  AND is_deleted = 0
  AND (user_id = ? OR text LIKE ?)
LIMIT 1
`
	var one int
	err := db.conn.QueryRow(q, workspaceID, channelID, threadTS, selfUserID, mention).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking thread involvement: %w", err)
	}
	return true, nil
}
