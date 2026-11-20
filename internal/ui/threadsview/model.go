// Package threadsview is the UI model for the "Threads" panel: a vertical
// list of threads the user is involved in, sourced from core.ThreadSummary,
// drawn the way Slack's Threads view draws them: a channel header and the
// thread's participants above a card holding the parent message, a "Show N
// more replies" link and the newest replies.
//
// The model is purely presentation: callers (typically the App layer) push
// new summaries via SetSummaries whenever the cache produces a fresh ranking,
// and read SelectedSummary / Selected to drive panel switching when the user
// activates a row.
package threadsview

import (
	"io"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/core"
	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/gammons/slk/internal/usergroups"
)

// threadKey identifies a thread across reloads that reorder the list.
type threadKey struct{ channelID, threadTS string }

// cardKey is everything a rendered card depends on.
type cardKey struct {
	summary  core.ThreadSummary
	width    int
	selected bool
	focused  bool
	last     bool
	gen      int64
	theme    int64
}

// renderedCard is one card's rows (ending in its spacer row, except the
// last card's) and the kitty uploads its emoji placements need.
type renderedCard struct {
	key     cardKey
	lines   []string
	flushes []func(io.Writer) error
}

// Model holds the threads-list state.
type Model struct {
	summaries    []core.ThreadSummary
	userNames    map[string]string
	channelNames map[string]string
	userGroups   map[string]string
	selfUserID   string

	avatarFn         messages.AvatarFunc
	emojiCtx         messages.EmojiContext
	coloredUsernames bool

	selected int
	focused  bool

	// topCard and topRow anchor the viewport: its first row is row
	// topRow of card topCard. Cards differ in height, so the anchor is
	// kept in card terms and only the cards near it are ever rendered.
	topCard, topRow int
	width           int

	// snappedSelection lets View() avoid snapping the viewport back to
	// the selected card on every render. While snappedSelection ==
	// selected, programmatic scrolls (ScrollUp/ScrollDown) are preserved.
	snappedSelection int
	hasSnapped       bool

	// subscriptionsAvailable tracks whether Slack's
	// subscriptions.thread.list call succeeded most recently. When
	// false, View renders a one-line "Threads list unavailable"
	// banner above the list/empty-state. Default is true (optimistic).
	subscriptionsAvailable bool

	cards map[threadKey]renderedCard
	gen   int64

	// rowCards maps each row of the last frame to the card drawn on it,
	// or -1 for chrome, spacer and filler rows. ClickAt reads it.
	rowCards []int

	version int64
}

// New creates an empty Model. userNames is the user-id -> display-name map
// used to resolve mention IDs and authors. selfUserID is the current
// user's ID; the participants line names that user "you".
func New(userNames map[string]string, selfUserID string) Model {
	if userNames == nil {
		userNames = map[string]string{}
	}
	return Model{
		userNames:              userNames,
		selfUserID:             selfUserID,
		channelNames:           map[string]string{},
		subscriptionsAvailable: true,
		cards:                  map[threadKey]renderedCard{},
	}
}

// Version returns a counter that increments any time View() output could
// change. App's panel-output cache uses this to reuse rendered frames.
func (m *Model) Version() int64 { return m.version }

// dirty marks the next View as different from the last.
func (m *Model) dirty() { m.version++ }

// InvalidateCache drops every rendered card, for a change that reaches
// all of them (names, emoji, avatars, theme options).
func (m *Model) InvalidateCache() {
	m.gen++
	m.dirty()
}

// SetUserNames replaces the user id -> display name map. No-op (no version
// bump) when the new map has the same length and the same key/value pairs as
// the current one — required so the App-level panel cache can hit on idle
// re-renders.
func (m *Model) SetUserNames(names map[string]string) {
	if names == nil {
		names = map[string]string{}
	}
	if stringMapsEqual(m.userNames, names) {
		return
	}
	m.userNames = names
	m.InvalidateCache()
}

// PatchUserName records a display name resolved after SetUserNames and
// re-renders when a card shows that user (as an author, a participant, or a
// <@id> mention). The map is normally the App's shared one, which already
// holds the name, so the bump is decided by the cards rather than by a map
// change; unrelated names, which resolve in bursts, cost no re-render.
func (m *Model) PatchUserName(userID, displayName string) {
	if userID == "" {
		return
	}
	if m.userNames == nil {
		m.userNames = map[string]string{}
	}
	m.userNames[userID] = displayName
	for _, s := range m.summaries {
		if showsUser(s, userID) {
			m.InvalidateCache()
			return
		}
	}
}

// showsUser reports whether s's card names userID: as an author, a
// participant, or a <@id> mention in the parent or a previewed reply.
func showsUser(s core.ThreadSummary, userID string) bool {
	mention := "<@" + userID
	if s.ParentUserID == userID || s.LastReplyBy == userID || strings.Contains(s.ParentText, mention) {
		return true
	}
	if slices.Contains(s.ParticipantIDs(), userID) {
		return true
	}
	return slices.ContainsFunc(s.PreviewReplies(), func(r core.ThreadReply) bool {
		return r.UserID == userID || strings.Contains(r.Text, mention)
	})
}

// stringMapsEqual reports whether two map[string]string have identical
// contents. Used by SetUserNames and SetChannelNames to short-circuit Set*
// calls with unchanged input so the App-level panel cache can hit on idle
// re-renders.
//
// Hand-rolled (rather than reflect.DeepEqual) because this runs on the render
// hot path — every threadsview render checks it. Please don't "simplify" this
// to reflect.DeepEqual without re-benchmarking.
func stringMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || vb != va {
			return false
		}
	}
	return true
}

// SetChannelNames sets the channel ID -> name map: bare <#CHANNELID>
// mentions and the header of DM and group DM threads, whose cached
// channel name is empty or Slack's raw mpdm form. No-op when the new map
// matches the current one.
func (m *Model) SetChannelNames(names map[string]string) {
	if names == nil {
		names = map[string]string{}
	}
	if stringMapsEqual(m.channelNames, names) {
		return
	}
	m.channelNames = names
	m.InvalidateCache()
}

// SetUserGroups sets the workspace-scoped usergroup ID -> handle map used
// to resolve bare <!subteam^SID> mentions in thread previews. No-op when
// the new map matches the current one.
func (m *Model) SetUserGroups(groups map[string]string) {
	if usergroups.Equal(m.userGroups, groups) {
		return
	}
	copied := usergroups.Copy(groups)
	if copied == nil {
		copied = map[string]string{}
	}
	m.userGroups = copied
	m.InvalidateCache()
}

// SetSelfUserID updates the current user's ID, named "you" among the
// participants.
func (m *Model) SetSelfUserID(id string) {
	if m.selfUserID != id {
		m.selfUserID = id
		m.InvalidateCache()
	}
}

// SetAvatarFunc sets the avatar drawn beside each message; nil draws none.
func (m *Model) SetAvatarFunc(fn messages.AvatarFunc) {
	m.avatarFn = fn
	m.InvalidateCache()
}

// HandleAvatarReady re-renders once a lazily fetched avatar lands.
func (m *Model) HandleAvatarReady(userID string) {
	if userID == "" || m.avatarFn == nil {
		return
	}
	m.InvalidateCache()
}

// SetEmojiContext sets how emoji in message text render (images or glyphs).
func (m *Model) SetEmojiContext(ctx messages.EmojiContext) {
	m.emojiCtx = ctx
	m.InvalidateCache()
}

// SetEmojiCustoms replaces only the workspace's custom emoji map.
func (m *Model) SetEmojiCustoms(customs map[string]string) {
	m.emojiCtx.Customs = customs
	m.InvalidateCache()
}

// HandleEmojiImageReady re-renders once an emoji image lands.
func (m *Model) HandleEmojiImageReady() { m.InvalidateCache() }

// SetColoredUsernames colors each author's name by user ID, as the
// message panes do with [appearance] colored_usernames.
func (m *Model) SetColoredUsernames(on bool) {
	if m.coloredUsernames != on {
		m.coloredUsernames = on
		m.InvalidateCache()
	}
}

// SetFocused marks whether the panel currently has keyboard focus. The
// selected card's border takes the focused selection colour.
func (m *Model) SetFocused(f bool) {
	if m.focused != f {
		m.focused = f
		m.dirty()
	}
}

// Focused reports whether the panel currently has keyboard focus.
func (m *Model) Focused() bool { return m.focused }

// SetSubscriptionsAvailable records whether Slack's authoritative
// subscription state could be fetched most recently. false flips the
// "Threads list unavailable" banner on; true clears it.
func (m *Model) SetSubscriptionsAvailable(available bool) {
	if m.subscriptionsAvailable == available {
		return
	}
	m.subscriptionsAvailable = available
	m.dirty()
}

// SetSummaries replaces the list of thread summaries. If the previously-
// selected (channelID, threadTS) pair is still present in the new list, the
// selection follows it to its new position; otherwise the selection resets
// to the top.
//
// Identical contents are a no-op (no version bump, no re-snap): the list
// is reloaded on every thread reply and mark-read, usually unchanged, and
// each bump forces a re-render of the panel.
func (m *Model) SetSummaries(s []core.ThreadSummary) {
	if slices.Equal(m.summaries, s) {
		return
	}
	prevCh, prevTS, hadSel := m.selectedKey()
	m.summaries = s

	newSel := 0
	if hadSel {
		for i, t := range s {
			if t.ChannelID == prevCh && t.ThreadTS == prevTS {
				newSel = i
				break
			}
		}
	}
	m.selected = newSel
	m.clampSelection()
	m.pruneCards()
	m.topCard, m.topRow = min(m.topCard, max(len(s)-1, 0)), 0
	m.hasSnapped = false
	m.dirty()
}

// pruneCards drops rendered cards for threads no longer listed.
func (m *Model) pruneCards() {
	if m.cards == nil {
		m.cards = map[threadKey]renderedCard{}
	}
	if len(m.cards) == 0 {
		return
	}
	live := make(map[threadKey]bool, len(m.summaries))
	for _, s := range m.summaries {
		live[threadKey{s.ChannelID, s.ThreadTS}] = true
	}
	for k := range m.cards {
		if !live[k] {
			delete(m.cards, k)
		}
	}
}

// Summaries returns the current list of thread summaries.
func (m *Model) Summaries() []core.ThreadSummary { return m.summaries }

// SelectedIndex returns the selection cursor's position, or 0 when the list
// is empty.
func (m *Model) SelectedIndex() int { return m.selected }

// Selected returns the (channelID, threadTS) of the currently selected row,
// with ok=false if the list is empty.
func (m *Model) Selected() (channelID, threadTS string, ok bool) {
	s, ok := m.SelectedSummary()
	if !ok {
		return "", "", false
	}
	return s.ChannelID, s.ThreadTS, true
}

// SelectedSummary returns the currently selected ThreadSummary.
func (m *Model) SelectedSummary() (core.ThreadSummary, bool) {
	if len(m.summaries) == 0 || m.selected < 0 || m.selected >= len(m.summaries) {
		return core.ThreadSummary{}, false
	}
	return m.summaries[m.selected], true
}

// selectedKey returns the (channelID, threadTS) pair currently selected,
// with ok=false when the list is empty. Used by SetSummaries to re-anchor
// selection across re-rankings.
func (m *Model) selectedKey() (string, string, bool) {
	s, ok := m.SelectedSummary()
	if !ok {
		return "", "", false
	}
	return s.ChannelID, s.ThreadTS, true
}

// MoveDown advances the cursor by one card, clamping at the bottom.
func (m *Model) MoveDown() {
	if m.selected < len(m.summaries)-1 {
		m.selected++
		m.dirty()
	}
}

// MoveUp moves the cursor up by one card, clamping at zero.
func (m *Model) MoveUp() {
	if m.selected > 0 {
		m.selected--
		m.dirty()
	}
}

// GoToTop jumps to the first card.
func (m *Model) GoToTop() {
	if m.selected != 0 {
		m.selected = 0
		m.dirty()
	}
}

// GoToBottom jumps to the last card.
func (m *Model) GoToBottom() {
	if n := len(m.summaries); n > 0 && m.selected != n-1 {
		m.selected = n - 1
		m.dirty()
	}
}

// ScrollUp moves the viewport up n rows without changing the selection.
// Marks the current selection as already-snapped so the next View() leaves
// the viewport alone; the next selection move (j/k/click) re-snaps.
func (m *Model) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	m.moveAnchor(-n)
	m.snappedSelection = m.selected
	m.hasSnapped = true
	m.dirty()
}

// ScrollDown moves the viewport down n rows without changing the
// selection. View() stops it at the end of the list. Marks the current
// selection as already-snapped (see ScrollUp).
func (m *Model) ScrollDown(n int) {
	if n <= 0 {
		return
	}
	m.moveAnchor(n)
	m.snappedSelection = m.selected
	m.hasSnapped = true
	m.dirty()
}

// ViewportAtTop reports whether the viewport is scrolled to the very top.
func (m *Model) ViewportAtTop() bool {
	return m.topCard == 0 && m.topRow == 0 && len(m.summaries) > 0
}

// ClickAt selects the thread card drawn on panel row rowY of the last
// frame (paneY as returned by App.panelAt: top border already stripped).
// Returns true when a card was selected and the caller should follow up
// with the open-thread command; false for the title, the banner, the
// spacer rows between cards, the filler past the last card, and rows
// outside the frame.
func (m *Model) ClickAt(rowY int) bool {
	if rowY < 0 || rowY >= len(m.rowCards) {
		return false
	}
	idx := m.rowCards[rowY]
	if idx < 0 || idx >= len(m.summaries) {
		return false
	}
	if m.selected != idx {
		m.selected = idx
		m.dirty()
	}
	return true
}

// MarkSelectedRead clears the local Unread flag on the currently selected
// summary, if any. Returns true when a flag was actually flipped (so callers
// can refresh dependent state, e.g. the sidebar's threads-row badge). This
// is a presentation-only update: it does not touch Slack server state and
// does not advance the thread_subscriptions row's last_read. The next
// refresh from cache.ListSubscribedThreads will recompute Unread from the
// persisted per-thread LastRead; a subsequent thread_marked WS echo (or
// an explicit MarkThreadRead call) is what durably clears it.
func (m *Model) MarkSelectedRead() bool {
	if m.selected < 0 || m.selected >= len(m.summaries) {
		return false
	}
	if !m.summaries[m.selected].Unread {
		return false
	}
	m.summaries[m.selected].Unread = false
	m.dirty()
	return true
}

// MarkByThreadTSUnread sets the local Unread flag on the summary matching
// (channelID, threadTS) to true. Returns true when a flag was actually
// flipped (i.e., the row existed and was previously read). Like
// MarkByThreadTSReadAt this is presentation-only: it does not touch Slack
// server state. Used by the U-key mark-unread flow and by the inbound
// thread_marked WS handler.
//
// Note: the next refresh from cache.ListSubscribedThreads recomputes
// Unread from the thread_subscriptions row's per-thread LastRead. If
// that value is still ahead of LastReplyTS (e.g. because a stale
// thread_marked persisted it), the flag may flip back to read on
// refresh.
func (m *Model) MarkByThreadTSUnread(channelID, threadTS string) bool {
	if channelID == "" || threadTS == "" {
		return false
	}
	for i := range m.summaries {
		if m.summaries[i].ChannelID == channelID && m.summaries[i].ThreadTS == threadTS {
			if m.summaries[i].Unread {
				return false
			}
			m.summaries[i].Unread = true
			m.dirty()
			return true
		}
	}
	return false
}

// MarkByThreadTSReadAt sets the Unread flag on the summary matching
// (channelID, threadTS) by comparing the thread's read cursor against
// its newest known reply: unread iff LastReplyTS > lastRead. Returns
// true only when the flag actually changed, so callers can skip the
// sidebar badge refresh.
//
// This is the correct handler for an inbound thread_marked echo, which
// carries a cursor. Slack's subscription `active` flag means
// "subscribed", not "unread", and must never be used for this decision.
// Presentation-only: the next cache.ListSubscribedThreads refresh
// recomputes Unread from the same rule against durable state.
func (m *Model) MarkByThreadTSReadAt(channelID, threadTS, lastRead string) bool {
	if channelID == "" || threadTS == "" {
		return false
	}
	for i := range m.summaries {
		if m.summaries[i].ChannelID != channelID || m.summaries[i].ThreadTS != threadTS {
			continue
		}
		want := m.summaries[i].LastReplyTS > lastRead
		if m.summaries[i].Unread == want {
			return false
		}
		m.summaries[i].Unread = want
		m.dirty()
		return true
	}
	return false
}

// UnreadCount returns the number of summaries currently flagged as unread.
func (m *Model) UnreadCount() int {
	n := 0
	for _, s := range m.summaries {
		if s.Unread {
			n++
		}
	}
	return n
}

// clampSelection keeps the cursor on a listed card.
func (m *Model) clampSelection() {
	if m.selected < 0 {
		m.selected = 0
	}
	if n := len(m.summaries); n == 0 {
		m.selected = 0
	} else if m.selected >= n {
		m.selected = n - 1
	}
}

// View renders the threads list to a string of `height` lines, each
// `width` columns wide. Argument order matches sidebar.View and
// thread.View (height first).
func (m *Model) View(height, width int) string {
	width = max(width, 1)
	height = max(height, 1)
	if width != m.width {
		m.topRow = 0
	}
	m.width = width

	var rows []string
	m.rowCards = m.rowCards[:0]
	add := func(line string, card int) {
		rows = append(rows, line)
		m.rowCards = append(m.rowCards, card)
	}

	if !m.subscriptionsAvailable {
		add(m.banner(width), -1)
	}
	add(m.title(width), -1)
	add(blankLine(width), -1)

	bodyHeight := max(height-len(rows), 0)
	if len(m.summaries) == 0 {
		empty := lipgloss.NewStyle().Foreground(styles.TextMuted).Background(styles.Background).Render("No threads yet")
		if bodyHeight > 0 {
			for _, l := range strings.Split(lipgloss.Place(width, bodyHeight, lipgloss.Center, lipgloss.Center, empty), "\n") {
				add(l, -1)
			}
		}
		return m.frame(rows, height)
	}

	if !m.hasSnapped || m.snappedSelection != m.selected {
		m.snapToSelected(bodyHeight)
		m.snappedSelection = m.selected
		m.hasSnapped = true
	}
	m.clampAnchor(bodyHeight)

	for i, row := m.topCard, m.topRow; i < len(m.summaries) && len(rows) < height; i, row = i+1, 0 {
		c := m.card(i)
		for _, fl := range c.flushes {
			_ = fl(imgpkg.KittyOutput)
		}
		content := cardContentRows(c)
		for ; row < len(c.lines) && len(rows) < height; row++ {
			owner := i
			if row >= content {
				owner = -1
			}
			add(c.lines[row], owner)
		}
	}
	for len(rows) < height {
		add(blankLine(width), -1)
	}
	return m.frame(rows, height)
}

// frame joins rows cut to the pane's height; a pane shorter than the
// chrome above the list loses the rows that do not fit.
func (m *Model) frame(rows []string, height int) string {
	n := min(len(rows), height)
	m.rowCards = m.rowCards[:n]
	return strings.Join(rows[:n], "\n")
}

// banner is the "Threads list unavailable" row, clipped to width.
func (m *Model) banner(width int) string {
	text := "Threads list unavailable — Slack subscription state could not be fetched. slk will retry on the next reconnect."
	return lipgloss.NewStyle().Foreground(styles.TextMuted).Background(styles.Background).
		Width(width).MaxWidth(width).Render(clipToWidth(text, width))
}

// title is the "Threads" heading over the list, as in Slack.
func (m *Model) title(width int) string {
	t := lipgloss.NewStyle().Foreground(styles.TextPrimary).Background(styles.Background).Bold(true).Render("Threads")
	if n := m.UnreadCount(); n > 0 {
		t += lipgloss.NewStyle().Foreground(styles.TextMuted).Background(styles.Background).Render("  ·  " + pluralize(n, "unread thread", "unread threads"))
	}
	return lipgloss.NewStyle().Background(styles.Background).Width(width).Render(clipToWidth(" "+t, width))
}

// card returns card i rendered at the current width, from the cache when
// nothing it depends on has changed.
func (m *Model) card(i int) renderedCard {
	s := m.summaries[i]
	selected := i == m.selected
	key := cardKey{
		summary:  s,
		width:    m.width,
		selected: selected,
		focused:  selected && m.focused,
		last:     i == len(m.summaries)-1,
		gen:      m.gen,
		theme:    styles.Version(),
	}
	tk := threadKey{s.ChannelID, s.ThreadTS}
	if c, ok := m.cards[tk]; ok && c.key == key {
		return c
	}
	lines, flushes := m.renderCard(s, m.width, selected)
	if !key.last {
		lines = append(lines, blankLine(m.width))
	}
	c := renderedCard{key: key, lines: lines, flushes: flushes}
	if m.cards == nil {
		m.cards = map[threadKey]renderedCard{}
	}
	m.cards[tk] = c
	return c
}

// cardHeight is the number of rows card i occupies, spacer included.
func (m *Model) cardHeight(i int) int { return len(m.card(i).lines) }

// cardContentRows is the number of a card's rows that are the card itself
// rather than the spacer after it.
func cardContentRows(c renderedCard) int {
	if c.key.last {
		return len(c.lines)
	}
	return len(c.lines) - 1
}

// snapToSelected moves the viewport the least distance that shows the
// whole selected card, or its top when it is taller than the viewport.
func (m *Model) snapToSelected(height int) {
	sel := m.selected
	if sel < m.topCard || (sel == m.topCard && m.topRow > 0) {
		m.topCard, m.topRow = sel, 0
		return
	}
	selRows := cardContentRows(m.card(sel))
	bottom := -m.topRow
	for i := m.topCard; i < sel && bottom <= height; i++ {
		bottom += m.cardHeight(i)
	}
	if bottom+selRows <= height {
		return
	}
	m.topCard, m.topRow = sel, 0
	if selRows >= height {
		return
	}
	m.moveAnchor(-(height - selRows))
}

// clampAnchor keeps the anchor on a listed card and stops the viewport
// at the end of the list, so a scroll never leaves blank rows below the
// last card while there are cards above the top to show instead.
func (m *Model) clampAnchor(height int) {
	n := len(m.summaries)
	if m.topCard >= n {
		m.topCard, m.topRow = max(n-1, 0), 0
	}
	if m.topCard < 0 {
		m.topCard, m.topRow = 0, 0
	}
	m.topRow = min(max(m.topRow, 0), max(m.cardHeight(m.topCard)-1, 0))
	shown := -m.topRow
	for i := m.topCard; i < n && shown < height; i++ {
		shown += m.cardHeight(i)
	}
	if shown < height {
		m.moveAnchor(-(height - shown))
	}
}

// moveAnchor scrolls the anchor by delta rows (negative is up), stopping
// at the first row of the first card and the last row of the last card.
func (m *Model) moveAnchor(delta int) {
	n := len(m.summaries)
	if n == 0 || m.width <= 0 {
		return
	}
	for delta > 0 {
		rest := m.cardHeight(m.topCard) - m.topRow
		switch {
		case delta < rest:
			m.topRow += delta
			delta = 0
		case m.topCard < n-1:
			delta -= rest
			m.topCard, m.topRow = m.topCard+1, 0
		default:
			m.topRow = max(rest+m.topRow-1, 0)
			delta = 0
		}
	}
	for delta < 0 {
		switch {
		case -delta <= m.topRow:
			m.topRow += delta
			delta = 0
		case m.topCard > 0:
			delta += m.topRow
			m.topCard--
			m.topRow = m.cardHeight(m.topCard)
		default:
			m.topRow = 0
			delta = 0
		}
	}
}

// blankLine returns an exactly `width`-column-wide empty line, used for
// spacer and filler rows.
func blankLine(width int) string {
	return lipgloss.NewStyle().Background(styles.Background).Width(width).Render("")
}
