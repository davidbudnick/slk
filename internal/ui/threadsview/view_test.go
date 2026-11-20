package threadsview

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

// fixedNow pins "today" so day labels ("Yesterday at …") are stable.
var fixedNow = time.Date(2026, 11, 20, 12, 0, 0, 0, time.Local)

// tsAt returns the Slack ts of a moment d before fixedNow.
func tsAt(d time.Duration) string {
	return fmt.Sprintf("%d.000000", fixedNow.Add(-d).Unix())
}

// pinClock makes the package's day labels relative to fixedNow.
func pinClock(t *testing.T) {
	t.Helper()
	messages.SetNowFunc(func() time.Time { return fixedNow })
	t.Cleanup(func() { messages.SetNowFunc(nil) })
}

// names is the user map the view tests resolve authors with.
func names() map[string]string {
	return map[string]string{"U1": "alice", "U2": "bob", "U3": "cy", "U4": "dee", "U5": "eve", "USELF": "me.myself"}
}

// thread builds a summary whose parent U1 posted a day ago.
func thread(id string, replyCount int, replies ...core.ThreadReply) cache.ThreadSummary {
	s := cache.ThreadSummary{
		ChannelID: "C" + id, ChannelName: "chan-" + id, ChannelType: "channel",
		ThreadTS: id + ".000000", ParentUserID: "U1", ParentText: "parent " + id,
		ParentTS: tsAt(26 * time.Hour), ReplyCount: replyCount, LastReplyTS: tsAt(25 * time.Hour),
	}
	s.Participants[0] = "U1"
	copy(s.Replies[:], replies)
	return s
}

// manySummaries returns n distinct summaries of varying height.
func manySummaries(n int) []cache.ThreadSummary {
	out := make([]cache.ThreadSummary, n)
	for i := range out {
		s := thread(fmt.Sprintf("%04d", i), 1+i%4)
		s.ChannelType = []string{"channel", "private", "dm"}[i%3]
		s.ParentText = fmt.Sprintf("parent %d with *bold* and `code` and <@U2> mention", i)
		s.Unread = i%3 == 0
		if i%2 == 0 {
			s.Replies[0] = core.ThreadReply{UserID: "U2", Text: fmt.Sprintf("reply %d", i), TS: tsAt(25 * time.Hour)}
			s.Participants[1] = "U2"
		}
		out[i] = s
	}
	return out
}

// plainLines renders the view and returns its rows without styling.
func plainLines(m *Model, height, width int) []string {
	return strings.Split(ansi.Strip(m.View(height, width)), "\n")
}

// rowOf returns the index of the first row containing needle, or -1.
func rowOf(lines []string, needle string) int {
	for i, l := range lines {
		if strings.Contains(l, needle) {
			return i
		}
	}
	return -1
}

func TestView_DrawsTitleChannelParticipantsAndParent(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	s := thread("1", 0)
	s.Participants = [core.ThreadPreviewParticipants]string{"U1", "U2", "USELF"}
	s.ParentText = "hello world"
	m.SetSummaries([]cache.ThreadSummary{s})
	lines := plainLines(&m, 30, 70)

	title, channel, who, parent := rowOf(lines, "Threads"), rowOf(lines, "# chan-1"), rowOf(lines, "alice, bob and you"), rowOf(lines, "hello world")
	if title != 0 {
		t.Errorf("title row = %d, want 0:\n%s", title, strings.Join(lines, "\n"))
	}
	if channel < 0 || who != channel+1 {
		t.Errorf("participants (row %d) must sit under the channel (row %d):\n%s", who, channel, strings.Join(lines, "\n"))
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[who+1]), "╭") {
		t.Errorf("the card must open under the participants, got %q", lines[who+1])
	}
	if parent < 0 || !strings.Contains(lines[parent-1], "alice") || !strings.Contains(lines[parent-1], "Yesterday at ") {
		t.Errorf("parent body must sit under its author and day, got:\n%s", strings.Join(lines, "\n"))
	}
}

func TestView_ShowsNewestRepliesUnderHiddenCount(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	m.SetSummaries([]cache.ThreadSummary{thread("1", 6,
		core.ThreadReply{UserID: "U2", Text: "older reply", TS: tsAt(3 * time.Hour)},
		core.ThreadReply{UserID: "U3", Text: "newest reply", TS: tsAt(2 * time.Hour)},
	)})
	lines := plainLines(&m, 30, 70)
	parent, link, older, newest := rowOf(lines, "parent 1"), rowOf(lines, "Show 4 more replies"), rowOf(lines, "older reply"), rowOf(lines, "newest reply")
	if !(parent >= 0 && parent < link && link < older && older < newest) {
		t.Errorf("want parent < link < older < newest, got %d %d %d %d:\n%s", parent, link, older, newest, strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[newest-1], "cy") {
		t.Errorf("reply must sit under its author, got %q", lines[newest-1])
	}
}

func TestView_HiddenReplyCountIsSingularForOne(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	m.SetSummaries([]cache.ThreadSummary{thread("1", 2, core.ThreadReply{UserID: "U2", Text: "r", TS: tsAt(time.Hour)})})
	if out := ansi.Strip(m.View(30, 70)); !strings.Contains(out, "Show 1 more reply") || strings.Contains(out, "more replies") {
		t.Errorf("want \"Show 1 more reply\":\n%s", out)
	}
}

func TestView_NoLinkWhenEveryReplyIsShown(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	m.SetSummaries([]cache.ThreadSummary{thread("1", 2,
		core.ThreadReply{UserID: "U2", Text: "one", TS: tsAt(3 * time.Hour)},
		core.ThreadReply{UserID: "U3", Text: "two", TS: tsAt(2 * time.Hour)},
	)})
	if out := ansi.Strip(m.View(30, 70)); strings.Contains(out, "more repl") || strings.Contains(out, "Last reply") {
		t.Errorf("nothing is hidden, so no link:\n%s", out)
	}
}

func TestView_UncachedRepliesShowCountAndLastReply(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	m.SetSummaries([]cache.ThreadSummary{thread("1", 3)})
	out := ansi.Strip(m.View(30, 70))
	if !strings.Contains(out, "3 replies") || !strings.Contains(out, "Last reply yesterday at ") {
		t.Errorf("want the reply count and the last reply's day:\n%s", out)
	}
}

func TestView_LongBodyIsCapped(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	s := thread("1", 0)
	s.ParentText = strings.TrimSpace(strings.Repeat("line\n", 30))
	m.SetSummaries([]cache.ThreadSummary{s})
	lines := plainLines(&m, 60, 70)
	if got := strings.Count(strings.Join(lines, "\n"), "line"); got != parentMaxLines {
		t.Errorf("body rows = %d, want %d", got, parentMaxLines)
	}
	if rowOf(lines, "Show more") < 0 {
		t.Errorf("a cut body must end in \"Show more\":\n%s", strings.Join(lines, "\n"))
	}
}

func TestChannelTitle_NamesTheOtherPeopleInADM(t *testing.T) {
	m := New(names(), "USELF")
	dm := cache.ThreadSummary{ChannelID: "D1", ChannelType: "dm", ThreadTS: "1.0"}
	dm.Participants = [core.ThreadPreviewParticipants]string{"USELF", "U1"}
	group := cache.ThreadSummary{ChannelID: "G1", ChannelType: "group_dm", ChannelName: "mpdm-a--b--c-1", ThreadTS: "2.0"}
	group.Participants = [core.ThreadPreviewParticipants]string{"U1", "USELF", "U2"}
	self := cache.ThreadSummary{ChannelID: "D2", ChannelType: "dm", ThreadTS: "3.0"}
	self.Participants = [core.ThreadPreviewParticipants]string{"USELF"}
	for _, c := range []struct {
		s    cache.ThreadSummary
		want string
	}{{dm, "● alice"}, {group, "● alice, bob"}, {self, "● me.myself"}} {
		if got := ansi.Strip(m.channelTitle(c.s, false)); got != c.want {
			t.Errorf("channelTitle(%s) = %q, want %q", c.s.ChannelID, got, c.want)
		}
	}
	m.SetChannelNames(map[string]string{"D1": "Alice Liddell"})
	if got := ansi.Strip(m.channelTitle(dm, false)); got != "● Alice Liddell" {
		t.Errorf("a known conversation name wins, got %q", got)
	}
}

func TestChannelTitle_MarksUnreadAndPrivate(t *testing.T) {
	m := New(names(), "USELF")
	s := cache.ThreadSummary{ChannelID: "C1", ChannelName: "secret", ChannelType: "private", Unread: true}
	if got := ansi.Strip(m.channelTitle(s, false)); got != "◆ secret  ●" {
		t.Errorf("channelTitle = %q", got)
	}
}

func TestParticipantsLine(t *testing.T) {
	m := New(names(), "USELF")
	for _, c := range []struct {
		ids  []string
		want string
	}{
		{nil, ""},
		{[]string{"U1"}, "alice"},
		{[]string{"USELF"}, "you"},
		{[]string{"U1", "U2"}, "alice and bob"},
		{[]string{"USELF", "U1"}, "alice and you"},
		{[]string{"U1", "U2", "U3", "USELF"}, "alice, bob, cy and you"},
		{[]string{"U1", "U2", "U3", "U4", "U5", "USELF"}, "alice, bob, cy and 3 others"},
		{[]string{"UNKNOWN"}, "UNKNOWN"},
	} {
		var s cache.ThreadSummary
		copy(s.Participants[:], c.ids)
		if got := m.participantsLine(s); got != c.want {
			t.Errorf("participantsLine(%v) = %q, want %q", c.ids, got, c.want)
		}
	}
}

func TestWhenLabel(t *testing.T) {
	pinClock(t)
	clock := func(d time.Duration) string { return fixedNow.Add(-d).Format("3:04 PM") }
	for _, c := range []struct {
		ago  time.Duration
		want string
	}{
		{time.Hour, clock(time.Hour)},
		{26 * time.Hour, "Yesterday at " + clock(26*time.Hour)},
		{50 * time.Hour, fixedNow.Add(-50*time.Hour).Format("Monday") + " at " + clock(50*time.Hour)},
		{24 * 30 * time.Hour, fixedNow.Add(-24*30*time.Hour).Format("Jan 2") + " at " + clock(24*30*time.Hour)},
	} {
		if got := whenLabel(tsAt(c.ago)); got != c.want {
			t.Errorf("whenLabel(%v ago) = %q, want %q", c.ago, got, c.want)
		}
	}
	if got := whenLabel(""); got != "" {
		t.Errorf("whenLabel(\"\") = %q, want empty", got)
	}
}

func TestView_EmptyState(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	if out := ansi.Strip(m.View(40, 60)); !strings.Contains(out, "No threads yet") {
		t.Errorf("empty View should say there are no threads, got:\n%s", out)
	}
}

func TestView_ParentNotLoadedFallback(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	m.SetSummaries([]cache.ThreadSummary{{ChannelID: "C1", ChannelName: "general", ChannelType: "channel", ThreadTS: "1.000000"}})
	if out := ansi.Strip(m.View(40, 60)); !strings.Contains(out, "parent not loaded") {
		t.Errorf("View should render the parent-not-loaded fallback, got:\n%s", out)
	}
}

func TestView_BannerOnlyWhenSubscriptionsUnavailable(t *testing.T) {
	m := New(map[string]string{}, "USELF")
	if out := ansi.Strip(m.View(10, 80)); strings.Contains(out, "Threads list unavailable") {
		t.Errorf("no banner expected by default:\n%s", out)
	}
	m.SetSubscriptionsAvailable(false)
	if lines := plainLines(&m, 10, 80); !strings.Contains(lines[0], "Threads list unavailable") {
		t.Errorf("banner must be the first row with no summaries, got %q", lines[0])
	}
	m.SetSummaries(sampleSummaries())
	lines := plainLines(&m, 30, 80)
	if !strings.Contains(lines[0], "Threads list unavailable") || rowOf(lines, "# general") < 0 {
		t.Errorf("banner must sit above the list:\n%s", strings.Join(lines, "\n"))
	}
}

func TestView_EveryRowIsExactlyTheWidth(t *testing.T) {
	pinClock(t)
	for _, width := range []int{1, 8, 20, 57, 120} {
		for _, avatars := range []bool{false, true} {
			m := New(names(), "USELF")
			if avatars {
				m.SetAvatarFunc(func(string) string { return "AAAA\nAAAA" })
			}
			m.SetSubscriptionsAvailable(width%2 == 0)
			m.SetSummaries(manySummaries(12))
			for _, height := range []int{1, 7, 40} {
				lines := strings.Split(m.View(height, width), "\n")
				if len(lines) != height {
					t.Errorf("width %d avatars %v: %d rows, want %d", width, avatars, len(lines), height)
				}
				for i, l := range lines {
					if w := lipgloss.Width(l); w != width {
						t.Errorf("width %d avatars %v height %d: row %d is %d wide: %q", width, avatars, height, i, w, ansi.Strip(l))
					}
				}
			}
		}
	}
}

func TestView_AvatarSitsBesideEachMessage(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	m.SetAvatarFunc(func(id string) string {
		if id == "U1" {
			return "<U1>\n[U1]"
		}
		return ""
	})
	m.SetSummaries([]cache.ThreadSummary{thread("1", 1, core.ThreadReply{UserID: "U2", Text: "the reply", TS: tsAt(time.Hour)})})
	lines := plainLines(&m, 30, 70)
	head, body := rowOf(lines, "<U1> alice"), rowOf(lines, "[U1] parent 1")
	if head < 0 || body != head+1 {
		t.Errorf("avatar rows must flank the author and the body (rows %d, %d):\n%s", head, body, strings.Join(lines, "\n"))
	}
	if reply := rowOf(lines, "the reply"); !strings.Contains(lines[reply], "│      the reply") {
		t.Errorf("a message with no avatar yet keeps the avatar's columns, got %q", lines[reply])
	}
	v := m.Version()
	m.HandleAvatarReady("U2")
	if m.Version() == v {
		t.Error("a landed avatar must re-render the view")
	}
}

func TestView_SelectionChangesOutput(t *testing.T) {
	m := New(names(), "USELF")
	m.SetSummaries(sampleSummaries())
	before := m.View(40, 60)
	m.MoveDown()
	if after := m.View(40, 60); before == after {
		t.Errorf("View output unchanged after MoveDown; selection styling not applied")
	}
}

func TestView_OnlyTheSelectedCardTakesTheSelectionBorder(t *testing.T) {
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	m := New(names(), "USELF")
	m.SetFocused(true)
	m.SetSummaries(sampleSummaries())
	sel := lipgloss.NewStyle().Foreground(styles.SelectionBorderColor(true)).Render("╭")
	sgr := sel[:strings.Index(sel, "╭")]
	for _, want := range []int{0, 1} {
		lines := strings.Split(m.View(40, 60), "\n")
		var tops []int
		for i, l := range lines {
			if strings.Contains(l, "╭") && strings.Contains(l, strings.TrimSuffix(sgr, "m")) {
				tops = append(tops, i)
			}
		}
		plain := strings.Split(ansi.Strip(strings.Join(lines, "\n")), "\n")
		wantRow := rowOf(plain, []string{"# general", "# design"}[want]) + 1
		if len(tops) != 1 || tops[0] != wantRow {
			t.Errorf("selected %d: selection-coloured card tops at %v, want [%d]", want, tops, wantRow)
		}
		m.MoveDown()
	}
}

func TestView_SnapsToSelectedOnOverflow(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	m.SetSummaries(manySummaries(10))
	const height, width = 20, 60
	if lines := plainLines(&m, height, width); rowOf(lines, "chan-0000") < 0 {
		t.Fatalf("the first card must show initially:\n%s", strings.Join(lines, "\n"))
	}
	for i := 0; i < 9; i++ {
		m.MoveDown()
		lines := plainLines(&m, height, width)
		name := fmt.Sprintf("chan-%04d", i+1)
		top := rowOf(lines, name)
		if top < 0 {
			top = rowOf(lines, "● alice")
		}
		bottom := -1
		for r := max(top, 0); r < len(lines); r++ {
			if strings.Contains(lines[r], "╰") {
				bottom = r
				break
			}
		}
		if top < 0 || bottom < 0 {
			t.Fatalf("selected card %d is not wholly on screen (top %d, bottom %d):\n%s", i+1, top, bottom, strings.Join(lines, "\n"))
		}
	}
	m.GoToTop()
	if lines := plainLines(&m, height, width); rowOf(lines, "chan-0000") != 2 {
		t.Errorf("GoToTop must bring the first card under the title:\n%s", strings.Join(lines, "\n"))
	}
}

func TestView_TallSelectedCardPinsItsTop(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	tall := thread("2", 9,
		core.ThreadReply{UserID: "U2", Text: strings.TrimSpace(strings.Repeat("r\n", 9)), TS: tsAt(3 * time.Hour)},
		core.ThreadReply{UserID: "U3", Text: strings.TrimSpace(strings.Repeat("s\n", 9)), TS: tsAt(2 * time.Hour)},
	)
	tall.ParentText = strings.TrimSpace(strings.Repeat("p\n", 9))
	m.SetSummaries([]cache.ThreadSummary{thread("1", 0), tall})
	m.MoveDown()
	if lines := plainLines(&m, 12, 60); rowOf(lines, "# chan-2") != 2 {
		t.Errorf("a card taller than the viewport shows from its top:\n%s", strings.Join(lines, "\n"))
	}
}

func TestClickAt_SelectsTheCardUnderTheRow(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	m.SetSummaries([]cache.ThreadSummary{thread("1", 0), thread("2", 0), thread("3", 0)})
	lines := plainLines(&m, 40, 60)
	second := rowOf(lines, "# chan-2")
	for _, c := range []struct {
		name string
		row  int
		want bool
		sel  int
	}{
		{"title", 0, false, 0},
		{"blank under the title", 1, false, 0},
		{"spacer between cards", second - 1, false, 0},
		{"second card's channel row", second, true, 1},
		{"second card's body", rowOf(lines, "parent 2"), true, 1},
		{"third card's bottom border", len(lines) - 1 - countTrailingBlank(lines), true, 2},
		{"filler below the list", len(lines) - 1, false, 2},
		{"above the pane", -1, false, 2},
		{"below the pane", len(lines), false, 2},
	} {
		if got := m.ClickAt(c.row); got != c.want || m.SelectedIndex() != c.sel {
			t.Errorf("%s (row %d): ClickAt = %v, selected %d; want %v, %d", c.name, c.row, got, m.SelectedIndex(), c.want, c.sel)
		}
	}
}

// countTrailingBlank is the number of empty rows at the end of lines.
func countTrailingBlank(lines []string) int {
	n := 0
	for i := len(lines) - 1; i >= 0 && strings.TrimSpace(lines[i]) == ""; i-- {
		n++
	}
	return n
}

func TestClickAt_FollowsTheScrolledViewport(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	m.SetSummaries(manySummaries(10))
	_ = m.View(20, 60)
	m.ScrollDown(4)
	lines := plainLines(&m, 20, 60)
	row := rowOf(lines, "chan-0001")
	if row < 0 {
		t.Fatalf("the second card must be on screen after the scroll:\n%s", strings.Join(lines, "\n"))
	}
	if !m.ClickAt(row) || m.SelectedIndex() != 1 {
		t.Errorf("ClickAt(%d) selected %d, want card 1", row, m.SelectedIndex())
	}
	if !m.ClickAt(2) || m.SelectedIndex() != 0 {
		t.Errorf("the first body row still shows the tail of card 0; selected %d", m.SelectedIndex())
	}
}

func TestScroll_MovesByRowsAndStopsAtBothEnds(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	m.SetSummaries(manySummaries(6))
	const height, width = 20, 60
	top := plainLines(&m, height, width)
	if !m.ViewportAtTop() {
		t.Fatal("a fresh view starts at the top")
	}
	m.ScrollDown(3)
	down := plainLines(&m, height, width)
	if m.ViewportAtTop() || down[2] != top[5] {
		t.Errorf("ScrollDown(3) must move the list up three rows under the title; row 2 = %q, want %q", down[2], top[5])
	}
	if m.SelectedIndex() != 0 {
		t.Errorf("a scroll must not move the selection, got %d", m.SelectedIndex())
	}
	m.ScrollDown(10_000)
	end := plainLines(&m, height, width)
	if !strings.Contains(end[height-1], "╰") {
		t.Errorf("a scroll past the end stops with the last card's border on the last row, got %q", end[height-1])
	}
	m.ScrollUp(10_000)
	if back := plainLines(&m, height, width); !m.ViewportAtTop() || strings.Join(back, "\n") != strings.Join(top, "\n") {
		t.Errorf("a scroll past the top returns to the first frame")
	}
}

func TestView_ShortListLeavesNoScroll(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	m.SetSummaries([]cache.ThreadSummary{thread("1", 0)})
	top := m.View(40, 60)
	m.ScrollDown(5)
	if got := m.View(40, 60); got != top || !m.ViewportAtTop() {
		t.Error("a list shorter than the viewport must not scroll")
	}
}

func TestView_RendersOnlyTheCardsNearTheViewport(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	m.SetSummaries(manySummaries(1000))
	const height, width = 40, 100
	_ = m.View(height, width)
	if n := len(m.cards); n > height {
		t.Errorf("first frame rendered %d cards for a %d-row viewport", n, height)
	}
	m.GoToBottom()
	lines := plainLines(&m, height, width)
	if n := len(m.cards); n > 2*height {
		t.Errorf("jumping to the bottom rendered %d cards", n)
	}
	if !strings.Contains(lines[height-1], "╰") {
		t.Errorf("the last card must end on the last row, got %q", lines[height-1])
	}
}

func TestSetSummaries_IdenticalListDoesNotBumpVersion(t *testing.T) {
	m := New(names(), "USELF")
	m.SetSummaries(manySummaries(5))
	v := m.Version()
	m.SetSummaries(manySummaries(5))
	if m.Version() != v {
		t.Error("an identical list must not bump the version")
	}
}

func TestSetSummaries_IdenticalListPreservesScroll(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	m.SetSummaries(manySummaries(10))
	_ = m.View(20, 60)
	m.ScrollDown(7)
	want := m.View(20, 60)
	m.SetSummaries(manySummaries(10))
	if got := m.View(20, 60); got != want {
		t.Error("reloading an identical list must leave the viewport where it was")
	}
}

func TestSetSummaries_ChangedThreadRerendersOnlyItsCard(t *testing.T) {
	pinClock(t)
	m := New(names(), "USELF")
	list := manySummaries(3)
	m.SetSummaries(list)
	_ = m.View(60, 60)
	first := m.cards[threadKey{list[0].ChannelID, list[0].ThreadTS}].lines
	next := manySummaries(3)
	next[1].ParentText = "edited parent"
	m.SetSummaries(next)
	out := ansi.Strip(m.View(60, 60))
	if !strings.Contains(out, "edited parent") {
		t.Errorf("the changed thread must show its new text:\n%s", out)
	}
	if got := m.cards[threadKey{list[0].ChannelID, list[0].ThreadTS}].lines; &got[0] != &first[0] {
		t.Error("an unchanged card must be reused, not rendered again")
	}
}

func TestTitle_CountsUnreadThreads(t *testing.T) {
	m := New(names(), "USELF")
	m.SetSummaries(sampleSummaries())
	if lines := plainLines(&m, 30, 60); !strings.Contains(lines[0], "Threads  ·  1 unread thread") {
		t.Errorf("title = %q", lines[0])
	}
	m.MarkSelectedRead()
	if lines := plainLines(&m, 30, 60); strings.TrimSpace(lines[0]) != "Threads" {
		t.Errorf("title with nothing unread = %q", lines[0])
	}
}
