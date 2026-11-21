// Package activityview is the UI model for the "Activity" panel: a vertical
// list of items the user was notified about — @mentions, thread replies,
// reactions to their messages, and DMs — sourced from core.ActivityItem.
//
// It mirrors internal/ui/threadsview so the App layer can wire it
// symmetrically: callers push a fresh page via SetItems and read
// SelectedItem to drive panel switching when the user activates a row.
//
// Each item renders as Slack's feed draws it, a three-line card beside the
// author's avatar: line 1 is the author and a relative time; line 2 is the
// hydrated message-body preview (empty until the messages.list hydration
// lands — see SetBodies); line 3 says where it happened ("Direct message",
// "Thread in #ch", "Mention in #ch"). A rule separates one card from the
// next, and an "Activity" heading tops the list.
package activityview

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/muesli/reflow/truncate"
)

// Each activity row is a three-line card (line 1: author + time; line 2:
// the hydrated message-body preview; line 3: where it happened) followed
// by one separator line, except the last card. card i therefore starts at
// flat line i*cardStride.
const (
	cardContentLines = 3
	cardStride       = cardContentLines + 1
)

// chromeRows is the heading and the blank row under it, above the list.
const chromeRows = 2

// A card's text starts after its icon: the author's four-cell avatar and
// a space, or, when no avatars are drawn, the type glyph and a space.
const (
	avatarGutter = 5
	glyphGutter  = 2
)

// reminderHint fills line 2 of a Slack reminder's card: the feed carries
// no reminder text.
const reminderHint = "Open Slack to see this reminder"

// Local styles, kept package-private and built from the shared color tokens
// so theme changes propagate via styles.Apply().
func mutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.TextMuted)
}

func bodyStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.TextPrimary)
}

func unreadDotStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
}

// channelNameStyle themes the channel name so it stays readable on light
// themes, mirroring the sidebar's "channel link" convention.
func channelNameStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
}

// thickLeftBorder mirrors the messages/threadsview convention: a 1-column
// left border using "▌" that marks the selected row.
var thickLeftBorder = lipgloss.Border{Left: "▌"}

func borderInvisStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(styles.InvisibleLeftBorder()).BorderLeft(true).
		BorderForeground(styles.Background).
		BorderBackground(styles.Background)
}

func borderSelectStyle(focused bool) lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(thickLeftBorder).BorderLeft(true).
		BorderForeground(styles.SelectionBorderColor(focused)).
		BorderBackground(styles.SelectionTintColor(focused)).
		Background(styles.SelectionTintColor(focused))
}

func borderFillStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(styles.Background)
}

// Model holds the activity-list state.
type Model struct {
	items        []core.ActivityItem
	userNames    map[string]string
	channelNames map[string]string
	channelTypes map[string]string
	// dmPeers maps a DM's channel ID to the other person's user ID, whose
	// avatar heads that DM's card.
	dmPeers    map[string]string
	selfUserID string
	// avatarFn draws a card's avatar; nil keeps the type glyphs.
	avatarFn messages.AvatarFunc

	// bodies holds hydrated message text/author for each item ref,
	// keyed by core.ActivityMsgKey(channelID, ts). Filled by SetBodies
	// after the messages.list second fetch; a row renders ref-only until
	// its body arrives.
	bodies map[string]core.ActivityMessage

	// unreadOnly tracks the read/unread filter flag. The view only tracks
	// it and shows an indicator; the App re-fetches with unread_only=true
	// when it flips (the server does the filtering).
	unreadOnly bool

	selected int
	yOffset  int
	focused  bool

	// snappedSelection lets View() avoid snapping yOffset back to the
	// selected row on every render (see threadsview for the rationale).
	snappedSelection int
	hasSnapped       bool

	version int64
}

// New creates an empty Model. userNames resolves actor IDs to display names;
// selfUserID renders the current user as "me".
func New(userNames map[string]string, selfUserID string) Model {
	if userNames == nil {
		userNames = map[string]string{}
	}
	return Model{
		userNames:    userNames,
		selfUserID:   selfUserID,
		channelNames: map[string]string{},
		channelTypes: map[string]string{},
		bodies:       map[string]core.ActivityMessage{},
	}
}

// SetBodies installs hydrated message bodies (keyed by
// core.ActivityMsgKey) fetched via messages.list, and forces a re-render.
// Identical contents are a no-op: every feed load re-hydrates, usually to
// the same bodies.
func (m *Model) SetBodies(bodies map[string]core.ActivityMessage) {
	if bodies == nil {
		bodies = map[string]core.ActivityMessage{}
	}
	if maps.Equal(m.bodies, bodies) {
		return
	}
	m.bodies = bodies
	m.dirty()
}

// Version returns a counter that increments any time View() output could
// change. App's panel-output cache uses this to reuse rendered frames.
func (m *Model) Version() int64 { return m.version }

func (m *Model) dirty() { m.version++ }

// SetUserNames replaces the user id -> display name map. No-op (no version
// bump) when the new map is content-equal to the current one so the
// App-level panel cache can hit on idle re-renders.
func (m *Model) SetUserNames(names map[string]string) {
	if names == nil {
		names = map[string]string{}
	}
	if stringMapsEqual(m.userNames, names) {
		return
	}
	m.userNames = names
	m.dirty()
}

// PatchUserName records a display name resolved after SetUserNames and
// bumps the version when a card shows that user (as actor, hydrated
// author, or a <@id> mention in the body). The map is normally the App's
// shared one, which already holds the name, so the bump is decided by
// the cards rather than by a map change; unrelated names, which resolve
// in bursts, cost no re-render.
func (m *Model) PatchUserName(userID, displayName string) {
	if userID == "" {
		return
	}
	if m.userNames == nil {
		m.userNames = map[string]string{}
	}
	m.userNames[userID] = displayName
	mention := "<@" + userID
	for _, it := range m.items {
		body := m.bodies[core.ActivityMsgKey(it.ChannelID, it.TS)]
		if it.AuthorID == userID || body.UserID == userID || strings.Contains(body.Text, mention) {
			m.dirty()
			return
		}
	}
}

// SetChannelNames replaces the channel id -> name map. No-op when
// content-equal to the current one.
func (m *Model) SetChannelNames(names map[string]string) {
	if names == nil {
		names = map[string]string{}
	}
	if stringMapsEqual(m.channelNames, names) {
		return
	}
	m.channelNames = names
	m.dirty()
}

// SetChannelTypes replaces the channel id -> type map ("channel",
// "private", "dm", "group_dm"), which decides how a conversation is named
// on a card. No-op when content-equal to the current one.
func (m *Model) SetChannelTypes(types map[string]string) {
	if types == nil {
		types = map[string]string{}
	}
	if stringMapsEqual(m.channelTypes, types) {
		return
	}
	m.channelTypes = types
	m.dirty()
}

// SetDMPeers replaces the DM channel id -> other person's user id map.
// No-op when content-equal to the current one.
func (m *Model) SetDMPeers(peers map[string]string) {
	if stringMapsEqual(m.dmPeers, peers) {
		return
	}
	m.dmPeers = peers
	m.dirty()
}

// SetAvatarFunc draws each card's avatar beside it, as Slack does. Nil
// restores the type glyphs.
func (m *Model) SetAvatarFunc(fn messages.AvatarFunc) {
	m.avatarFn = fn
	m.dirty()
}

// HandleAvatarReady redraws the list once an avatar has loaded.
func (m *Model) HandleAvatarReady(string) {
	if m.avatarFn != nil && len(m.items) > 0 {
		m.dirty()
	}
}

// stringMapsEqual reports whether two map[string]string have identical
// contents. Hand-rolled (rather than reflect.DeepEqual) because it runs on
// the render hot path.
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

// SetSelfUserID updates the current user's ID. Used to render "me" labels.
func (m *Model) SetSelfUserID(id string) {
	if m.selfUserID != id {
		m.selfUserID = id
		m.dirty()
	}
}

// SetFocused marks whether the panel currently has keyboard focus.
func (m *Model) SetFocused(f bool) {
	if m.focused != f {
		m.focused = f
		m.dirty()
	}
}

// Focused reports whether the panel currently has keyboard focus.
func (m *Model) Focused() bool { return m.focused }

// UnreadOnly reports whether the unread-only filter is currently on.
func (m *Model) UnreadOnly() bool { return m.unreadOnly }

// ToggleUnreadOnly flips the unread-only filter flag and returns its new
// value. The App observes the returned value and re-fetches the feed with
// unread_only set accordingly; the view merely tracks the flag.
func (m *Model) ToggleUnreadOnly() bool {
	m.unreadOnly = !m.unreadOnly
	m.dirty()
	return m.unreadOnly
}

// SetItems replaces the list of activity items. If the previously-selected
// item (by Key) is still present, the selection follows it to its new
// position; otherwise the selection resets to the top.
//
// Identical contents are a no-op (no version bump, no re-snap): every
// activation of the view refetches the feed, usually unchanged.
func (m *Model) SetItems(items []core.ActivityItem) {
	if slices.Equal(m.items, items) {
		return
	}
	prevKey, hadSel := m.selectedKey()
	m.items = items

	newSel := 0
	if hadSel {
		for i, it := range items {
			if it.Key == prevKey {
				newSel = i
				break
			}
		}
	}
	m.selected = newSel
	m.clampSelection()
	m.hasSnapped = false // force re-snap on next render
	m.dirty()
}

// Items returns the current list of activity items.
func (m *Model) Items() []core.ActivityItem { return m.items }

// SelectedIndex returns the selection cursor's position, or 0 when empty.
func (m *Model) SelectedIndex() int { return m.selected }

// SelectedItem returns the currently selected ActivityItem (ChannelID / TS /
// ThreadTS carry enough to open the target), with ok=false when the list is
// empty.
func (m *Model) SelectedItem() (core.ActivityItem, bool) {
	if len(m.items) == 0 || m.selected < 0 || m.selected >= len(m.items) {
		return core.ActivityItem{}, false
	}
	return m.items[m.selected], true
}

// selectedKey returns the Key of the currently selected item, with ok=false
// when empty. Used by SetItems to re-anchor selection across refreshes.
func (m *Model) selectedKey() (string, bool) {
	it, ok := m.SelectedItem()
	if !ok {
		return "", false
	}
	return it.Key, true
}

// MoveDown advances the cursor by one row, clamping at the bottom.
func (m *Model) MoveDown() {
	if m.selected < len(m.items)-1 {
		m.selected++
		m.dirty()
	}
}

// MoveUp moves the cursor up by one row, clamping at zero.
func (m *Model) MoveUp() {
	if m.selected > 0 {
		m.selected--
		m.dirty()
	}
}

// GoToTop jumps to the first row.
func (m *Model) GoToTop() {
	if m.selected != 0 {
		m.selected = 0
		m.dirty()
	}
}

// GoToBottom jumps to the last row.
func (m *Model) GoToBottom() {
	if n := len(m.items); n > 0 && m.selected != n-1 {
		m.selected = n - 1
		m.dirty()
	}
}

// ScrollUp moves the viewport up n lines without changing the selection.
func (m *Model) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	m.yOffset -= n
	if m.yOffset < 0 {
		m.yOffset = 0
	}
	m.snappedSelection = m.selected
	m.hasSnapped = true
	m.dirty()
}

// ScrollDown moves the viewport down n lines without changing the selection.
// View() clamps yOffset against the actual content height.
func (m *Model) ScrollDown(n int) {
	if n <= 0 {
		return
	}
	m.yOffset += n
	m.snappedSelection = m.selected
	m.hasSnapped = true
	m.dirty()
}

// ViewportAtTop reports whether the viewport is scrolled to the very top.
func (m *Model) ViewportAtTop() bool {
	return m.yOffset == 0 && len(m.items) > 0
}

// ClickAt selects the activity row whose visual row contains rowY (the
// panel-local Y inside the bordered messages-pane content area). Returns
// true when a row was selected and the caller should follow up with the
// open command; false for separator rows, the blank-fill region past the
// last row, the heading, and negative rowY. A click on any line of a card
// maps to the same item via absLine / cardStride.
func (m *Model) ClickAt(rowY int) bool {
	rowY -= chromeRows
	if rowY < 0 {
		return false
	}
	absLine := m.yOffset + rowY
	if absLine%cardStride >= cardContentLines {
		return false
	}
	idx := absLine / cardStride
	if idx < 0 || idx >= len(m.items) {
		return false
	}
	if m.selected != idx {
		m.selected = idx
		m.dirty()
	}
	return true
}

// UnreadCount returns the number of items currently flagged unread.
func (m *Model) UnreadCount() int {
	n := 0
	for _, it := range m.items {
		if it.IsUnread {
			n++
		}
	}
	return n
}

// SetItemRead sets the read state of the item with the given key and
// reports whether there was one: the list may have been refetched since
// the caller took the item. The item stays in the list either way; with
// the unread-only filter on, it goes on the next fetch.
func (m *Model) SetItemRead(key string, read bool) bool {
	for i := range m.items {
		if m.items[i].Key != key {
			continue
		}
		if m.items[i].IsUnread == read {
			m.items[i].IsUnread = !read
			m.dirty()
		}
		return true
	}
	return false
}

func (m *Model) clampSelection() {
	if m.selected < 0 {
		m.selected = 0
	}
	if n := len(m.items); n == 0 {
		m.selected = 0
	} else if m.selected >= n {
		m.selected = n - 1
	}
}

// View renders the activity list to `height` lines, each `width` columns
// wide. Argument order matches sidebar.View / thread.View (height first).
func (m *Model) View(height, width int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	chrome := []string{m.title(width), blankLine(width)}
	if height <= chromeRows {
		return strings.Join(chrome[:height], "\n")
	}
	return strings.Join(chrome, "\n") + "\n" + m.list(height-chromeRows, width)
}

// title is the "Activity" heading over the list, as in Slack, with the
// unread count and the unread-only filter when it is on.
func (m *Model) title(width int) string {
	t := lipgloss.NewStyle().Foreground(styles.TextPrimary).Background(styles.Background).Bold(true).Render("Activity")
	note := ""
	if n := m.UnreadCount(); n > 0 {
		note += "  ·  " + strconv.Itoa(n) + " unread"
	}
	if m.unreadOnly {
		note += "  ·  unreads only"
	}
	t += mutedStyle().Background(styles.Background).Render(note)
	return lipgloss.NewStyle().Background(styles.Background).Width(width).Render(clipToWidth(" "+t, width))
}

// list renders the cards to `height` lines under the heading.
func (m *Model) list(height, width int) string {
	if len(m.items) == 0 {
		msg := "no activity"
		if m.unreadOnly {
			msg = "no unread activity"
		}
		empty := mutedStyle().Render(msg)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, empty)
	}

	// Cards have a fixed stride, so the scroll math needs only the line
	// count; render just the cards in the viewport.
	totalLines := len(m.items)*cardStride - 1
	if !m.hasSnapped || m.snappedSelection != m.selected {
		m.snapToSelected(height, totalLines)
		m.snappedSelection = m.selected
		m.hasSnapped = true
	}
	maxOffset := totalLines - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.yOffset > maxOffset {
		m.yOffset = maxOffset
	}
	if m.yOffset < 0 {
		m.yOffset = 0
	}
	end := m.yOffset + height
	if end > totalLines {
		end = totalLines
	}
	visible := m.renderLines(m.yOffset, end, width)
	if pad := height - len(visible); pad > 0 {
		filler := blankLine(width)
		out := make([]string, 0, height)
		out = append(out, visible...)
		for i := 0; i < pad; i++ {
			out = append(out, filler)
		}
		visible = out
	}
	return strings.Join(visible, "\n")
}

// snapToSelected adjusts yOffset so the selected card's content lines are
// fully inside the viewport.
func (m *Model) snapToSelected(height, totalLines int) {
	start := m.selected * cardStride
	end := start + cardContentLines

	if end > m.yOffset+height {
		m.yOffset = end - height
	}
	if start < m.yOffset {
		m.yOffset = start
	}
	if m.yOffset < 0 {
		m.yOffset = 0
	}
	maxOffset := totalLines - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.yOffset > maxOffset {
		m.yOffset = maxOffset
	}
}

// renderLines returns lines [start, end) of the flat line list -- each
// item's three-line card, with a rule between adjacent cards, so the
// windowing / snap / click math stays stride-based -- rendering only the
// cards that overlap that range.
func (m *Model) renderLines(start, end, width int) []string {
	if end <= start {
		return nil
	}
	out := make([]string, 0, end-start)
	var separator string
	for i := start / cardStride; i < len(m.items) && i*cardStride < end; i++ {
		base := i * cardStride
		for j, line := range m.renderCard(m.items[i], width, i == m.selected) {
			if abs := base + j; abs >= start && abs < end {
				out = append(out, line)
			}
		}
		if sep := base + cardContentLines; i < len(m.items)-1 && sep >= start && sep < end {
			if separator == "" {
				separator = ruleLine(width)
			}
			out = append(out, separator)
		}
	}
	return out
}

// blankLine returns an exactly `width`-column-wide empty line.
func blankLine(width int) string {
	return lipgloss.NewStyle().Width(width).Render("")
}

// ruleLine is the hairline between two cards, as in Slack's feed, clear
// of the selection border's column.
func ruleLine(width int) string {
	return lipgloss.NewStyle().Foreground(styles.Border).Background(styles.Background).
		Render(" " + strings.Repeat("─", max(width-1, 0)))
}

// renderCard returns the three lines for one activity item, as Slack's
// feed lays it out:
//
//	<avatar> <author>  <relTime>                              [●]
//	<avatar> <body preview>      (a reaction: "Reacted :emoji: to: …")
//	         <where>             ("Direct message", "Thread in #ch", …)
//
// The avatar is the author's, or the other person's on a DM card; with
// no avatars drawn the type glyph leads line 1 instead. The author is
// bold while the item is unread, which also puts a dot at the end of
// line 1. Selection is a left border (▌) and a tint on all three lines.
func (m *Model) renderCard(it core.ActivityItem, width int, selected bool) [cardContentLines]string {
	contentWidth := max(width-1, 1)

	body := m.bodies[core.ActivityMsgKey(it.ChannelID, it.TS)]

	// Author = the actor (reactor for reactions, message author
	// otherwise), falling back to the hydrated message's user when the
	// ref carried no author (thread_v2 / dm).
	authorID := it.AuthorID
	if authorID == "" {
		authorID = body.UserID
	}
	author := m.resolveUser(authorID)
	// A DM row is about the conversation, so it is headed by the other
	// person (the DM's name and avatar) rather than by whoever wrote
	// last, which is often you.
	avatarID := authorID
	if isDMItem(it.Type) {
		if name := m.channelNames[it.ChannelID]; name != "" {
			author = name
		}
		if peer := m.dmPeers[it.ChannelID]; peer != "" {
			avatarID = peer
		}
	}
	// A card nobody authored (a reminder, a type slk does not know) is
	// headed by what it is, and line 3 then only says where.
	headless := author == ""
	where := m.contextLabel(it, !headless && it.Type != "message_reaction")
	if headless {
		author = verbLabel(it.Type)
	}

	gutter := glyphGutter
	icon := [2]string{activityGlyph(it.Type) + " ", "  "}
	if m.avatarFn != nil {
		gutter = avatarGutter
		icon = [2]string{" " + activityGlyph(it.Type) + "   ", "     "}
		if avatarID != "" {
			if rows := strings.Split(m.avatarFn(avatarID), "\n"); len(rows) == 2 {
				icon = [2]string{rows[0] + " ", rows[1] + " "}
			}
		}
	}
	textWidth := max(contentWidth-gutter, 1)

	left := authorStyle(it.IsUnread).Render(author)
	if rel := formatRelTime(it.FeedTS); rel != "" {
		left += "  " + mutedStyle().Render(rel)
	}
	right := ""
	if it.IsUnread {
		right = unreadDotStyle().Render("●")
	}
	line1 := icon[0] + layoutLine(left, right, textWidth)

	// Render the body the way the Threads view and message pane do, then
	// fold it onto one line: a card has exactly cardContentLines lines,
	// so a newline in the message would desync the windowing/click math.
	opts := messages.RenderSlackMarkdownOpts{UserNames: m.userNames, ChannelNames: m.channelNames}
	preview := ""
	if body.Text != "" {
		preview = messages.RenderSlackMarkdownWith(body.Text, opts)
	}
	if it.Type == "message_reaction" {
		// "Reacted :emoji: to: <message>", less whatever is missing; a
		// card nobody heads already says "Reacted" on line 1.
		var parts []string
		if !headless {
			parts = append(parts, "Reacted")
		}
		if detail := activityDetail(it); detail != "" {
			parts = append(parts, strings.TrimRight(messages.RenderSlackMarkdownWith(detail, opts), " "))
		}
		if preview != "" {
			parts = append(parts, "to:", preview)
		}
		preview = strings.Join(parts, " ")
	}
	// The renderer leaves plain text unstyled, so set the base text colour
	// here (the message pane does the same via styles.MessageText). Only
	// the foreground: a background would paint over the selection tint.
	if it.Type == "saved_reminder" && preview == "" {
		preview = mutedStyle().Render(reminderHint)
	}
	preview = strings.ReplaceAll(preview, "\n", " ")
	if selected {
		// The renderer restores the theme background after each styled
		// span; on the selected card that must be the selection tint.
		preview = messages.RepaintBgToSelectionTint(preview, m.focused)
	}
	line2 := ""
	if isDMItem(it.Type) && preview != "" && m.selfUserID != "" && body.UserID == m.selfUserID {
		// Outside bodyStyle: its closing reset would drop the body's
		// text colour.
		line2 += mutedStyle().Render("You: ")
	}
	line2 = icon[1] + clipToWidth(line2+bodyStyle().Render(preview), textWidth)
	line3 := strings.Repeat(" ", gutter) + clipToWidth(where, textWidth)

	return [cardContentLines]string{
		m.wrapLine(line1, contentWidth, selected),
		m.wrapLine(line2, contentWidth, selected),
		m.wrapLine(line3, contentWidth, selected),
	}
}

// contextLabel renders line 3 of a card, where the activity happened:
// "Direct message" for a DM card, else the conversation ("#ch"), led by
// what happened there when withVerb is set ("Mention in #ch" / "Thread in
// #ch"). An unrecognised type is labelled with the type itself; see
// fallbackVerb.
func (m *Model) contextLabel(it core.ActivityItem, withVerb bool) string {
	if isDMItem(it.Type) {
		return mutedStyle().Render(contextVerb(it.Type))
	}
	verb := ""
	if withVerb {
		verb = verbLabel(it.Type)
	}
	ch := m.resolveChannel(it.ChannelID)
	chType := m.channelTypes[it.ChannelID]
	switch {
	case ch == "":
		return mutedStyle().Render(verb)
	case chType == "app" && verb == "":
		return mutedStyle().Render("App")
	case chType == "app":
		return mutedStyle().Render(verb + " in App")
	case chType == "dm" && verb == "":
		return mutedStyle().Render(contextVerb("dm"))
	case chType == "dm":
		// The other person is already on the card as the author.
		return mutedStyle().Render(verb + " in DM")
	}
	name := "#" + ch
	if chType == "private" || chType == "group_dm" {
		name = messages.ChannelGlyph(chType) + " " + ch
	}
	if verb != "" {
		verb += " in "
	}
	return mutedStyle().Render(verb) + channelNameStyle().Render(name)
}

// verbLabel is an item type's Activity-feed verb, or the type itself
// when slk does not know it.
func verbLabel(itemType string) string {
	if verb := contextVerb(itemType); verb != "" {
		return verb
	}
	return fallbackVerb(itemType)
}

// isDMItem reports whether an activity type is a DM row, which is headed
// by the conversation rather than by the latest message's sender.
func isDMItem(itemType string) bool {
	return itemType == "dm" || itemType == "bot_dm_bundle"
}

// contextVerb maps an item type to its Activity-feed verb.
func contextVerb(itemType string) string {
	switch itemType {
	case "at_user", "at_user_group", "at_channel", "at_everyone",
		"keyword", "list_user_mentioned", "unjoined_channel_mention", "channel":
		return "Mention"
	case "thread_v2":
		return "Thread"
	case "message_reaction":
		return "Reacted"
	case "dm", "bot_dm_bundle":
		return "Direct message"
	case "saved_reminder":
		return "Reminder"
	default:
		return ""
	}
}

// fallbackVerb labels an item type contextVerb does not know, from the
// type itself ("saved_reminder" -> "Saved reminder"). Such an item may
// carry no message ref, leaving the card no author and no body, so
// without this label it rendered as a blank row (issue #285).
func fallbackVerb(itemType string) string {
	if itemType == "" {
		return "Unknown activity"
	}
	s := strings.ReplaceAll(itemType, "_", " ")
	return strings.ToUpper(s[:1]) + s[1:]
}

// authorStyle renders the actor name, bold while the item is unread, as
// Slack does.
func authorStyle(unread bool) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.TextPrimary).Bold(unread)
}

// layoutLine places `right` flush against the right edge of a width-column
// line with `left` on the left, padding between. When there's no room for
// both, the right meta is dropped and the left is clipped. Widths are
// measured with lipgloss so ANSI styling doesn't skew the math.
func layoutLine(left, right string, width int) string {
	if right == "" {
		return clipToWidth(left, width)
	}
	lw := lipgloss.Width(left)
	rw := lipgloss.Width(right)
	if lw+1+rw > width {
		return clipToWidth(left, width)
	}
	return left + strings.Repeat(" ", width-lw-rw) + right
}

// wrapLine clips a rendered line to contentWidth and applies the selection
// border + background fill (shared by both card lines so selection spans
// the whole card).
func (m *Model) wrapLine(line string, contentWidth int, selected bool) string {
	line = clipToWidth(line, contentWidth)
	// Each styled span ends in a reset that drops the row's background;
	// restore it so the row is filled edge to edge.
	bg := messages.BgANSI()
	if selected {
		bg = messages.SelectionTintBgANSI(m.focused)
	}
	line = messages.ReapplyBgAfterResets(line, bg)
	borderStyle := borderInvisStyle()
	fill := borderFillStyle().Width(contentWidth)
	if selected {
		borderStyle = borderSelectStyle(m.focused)
		fill = lipgloss.NewStyle().
			Background(styles.SelectionTintColor(m.focused)).
			Width(contentWidth)
	}
	return borderStyle.Render(fill.Render(line))
}

// activityGlyph returns the leading glyph for a row keyed on the item type:
// "@" for mentions, the sidebar's thread flag for thread replies, a star
// for reactions, the sidebar's DM dot for DMs, and a clock face for
// reminders. Each is one column wide
// and has no colour-emoji form, so terminals agree on its width. Pure (no
// styling) so it can be unit-tested.
func activityGlyph(itemType string) string {
	switch itemType {
	case "at_user", "at_user_group", "at_channel", "at_everyone",
		"keyword", "list_user_mentioned", "unjoined_channel_mention", "channel":
		return "@"
	case "thread_v2":
		return "⚑"
	case "message_reaction":
		return "✦"
	case "dm", "bot_dm_bundle":
		return "●"
	case "saved_reminder":
		return "◷"
	default:
		return "•"
	}
}

// activityDetail returns a short detail (currently the reaction emoji
// ":name:") that renderCard prefixes to the body preview for
// message_reaction items. Pure so it can be unit-tested.
func activityDetail(it core.ActivityItem) string {
	if it.Type == "message_reaction" && it.Reaction != "" {
		return ":" + it.Reaction + ":"
	}
	return ""
}

// resolveChannel maps a channel ID to its cached name (falling back to the
// raw ID when uncached — acceptable for MVP; a mention can reference a
// channel slk hasn't cached).
func (m *Model) resolveChannel(id string) string {
	if id == "" {
		return ""
	}
	if name, ok := m.channelNames[id]; ok && name != "" {
		return name
	}
	return id
}

// resolveUser maps a Slack user ID to a display label: "me" for the current
// user, the cached display name when known, and the raw ID otherwise.
func (m *Model) resolveUser(uid string) string {
	if uid == "" {
		return ""
	}
	if uid == m.selfUserID {
		return "me"
	}
	if name, ok := m.userNames[uid]; ok && name != "" {
		return name
	}
	return uid
}

// formatRelTime parses a Slack-style "1700000000.000000" timestamp and
// returns a coarse age worded as Slack's feed words it: "1 min" /
// "5 mins" / "2 hrs" / "3 days". Empty / unparseable inputs return "".
func formatRelTime(ts string) string {
	if ts == "" {
		return ""
	}
	secStr := ts
	if dot := strings.IndexByte(ts, '.'); dot >= 0 {
		secStr = ts[:dot]
	}
	sec, err := strconv.ParseInt(secStr, 10, 64)
	if err != nil {
		return ""
	}
	return relAge(time.Since(time.Unix(sec, 0)))
}

// relAge words an age as Slack's feed does.
func relAge(d time.Duration) string {
	count := func(n time.Duration, unit string) string {
		if n == 1 {
			return "1 " + unit
		}
		return strconv.Itoa(int(n)) + " " + unit + "s"
	}
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return count(d/time.Minute, "min")
	case d < 24*time.Hour:
		return count(d/time.Hour, "hr")
	default:
		return count(d/(24*time.Hour), "day")
	}
}

// clipToWidth truncates an already-styled string to at most `width` display
// columns (trailing ellipsis) without padding short strings.
func clipToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	return truncate.StringWithTail(s, uint(width), "…")
}
