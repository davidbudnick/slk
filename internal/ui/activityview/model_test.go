package activityview

import (
	"image/color"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/emoji"
	"github.com/gammons/slk/internal/ui/styles"
)

func TestActivityGlyph(t *testing.T) {
	cases := map[string]string{
		"at_user":                  "@",
		"at_channel":               "@",
		"at_everyone":              "@",
		"at_user_group":            "@",
		"keyword":                  "@",
		"list_user_mentioned":      "@",
		"unjoined_channel_mention": "@",
		"channel":                  "@",
		"thread_v2":                "⚑",
		"message_reaction":         "✦",
		"dm":                       "●",
		"bot_dm_bundle":            "●",
		"saved_reminder":           "◷",
		"something_unknown":        "•",
		"":                         "•",
	}
	for typ, want := range cases {
		if got := activityGlyph(typ); got != want {
			t.Errorf("activityGlyph(%q) = %q, want %q", typ, got, want)
		}
	}
}

// Card glyphs must be one column wide and must not be characters a
// terminal may draw as a colour emoji. Those (✉, ☺) are counted as one
// column but drawn as two by terminals with an emoji font, which pushed
// "DM" into the glyph ("✉DM").
func TestActivityGlyphs_SingleCellAndNotEmoji(t *testing.T) {
	emojiGlyphs := map[string]bool{}
	for _, g := range emoji.CodeMap() {
		emojiGlyphs[strings.TrimSuffix(g, "\ufe0f")] = true
	}
	if !emojiGlyphs["✉"] || !emojiGlyphs["☺"] {
		t.Fatal("emoji.CodeMap no longer lists ✉ and ☺; this test cannot detect emoji-capable glyphs")
	}
	for _, typ := range []string{"at_user", "thread_v2", "message_reaction", "dm", "bot_dm_bundle", "saved_reminder", ""} {
		g := activityGlyph(typ)
		if w := emoji.Width(g); w != 1 {
			t.Errorf("glyph %q for %q is %d columns wide, want 1", g, typ, w)
		}
		if emojiGlyphs[g] {
			t.Errorf("glyph %q for %q can be drawn as a colour emoji", g, typ)
		}
	}
}

func TestActivityDetail(t *testing.T) {
	cases := []struct {
		name string
		it   core.ActivityItem
		want string
	}{
		{"reaction with name", core.ActivityItem{Type: "message_reaction", Reaction: "tada"}, ":tada:"},
		{"reaction without name", core.ActivityItem{Type: "message_reaction"}, ""},
		{"mention has no detail", core.ActivityItem{Type: "at_user", Reaction: "tada"}, ""},
		{"dm has no detail", core.ActivityItem{Type: "dm"}, ""},
	}
	for _, c := range cases {
		if got := activityDetail(c.it); got != c.want {
			t.Errorf("%s: activityDetail() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFormatRelTime(t *testing.T) {
	if got := formatRelTime(""); got != "" {
		t.Errorf("formatRelTime(empty) = %q, want empty", got)
	}
	if got := formatRelTime("not-a-ts"); got != "" {
		t.Errorf("formatRelTime(garbage) = %q, want empty", got)
	}
	for d, want := range map[time.Duration]string{
		30 * time.Second: "now",
		time.Minute:      "1 min",
		5 * time.Minute:  "5 mins",
		time.Hour:        "1 hr",
		3 * time.Hour:    "3 hrs",
		24 * time.Hour:   "1 day",
		72 * time.Hour:   "3 days",
	} {
		if got := relAge(d); got != want {
			t.Errorf("relAge(%s) = %q, want %q, as Slack words it", d, got, want)
		}
	}
}

func TestSetItemsPreservesSelectionByKey(t *testing.T) {
	m := New(nil, "")
	m.SetItems([]core.ActivityItem{
		{Key: "a"}, {Key: "b"}, {Key: "c"},
	})
	m.MoveDown()
	m.MoveDown() // select "c"
	if idx := m.SelectedIndex(); idx != 2 {
		t.Fatalf("SelectedIndex = %d, want 2", idx)
	}
	// Re-order; selection should follow "c" to its new position.
	m.SetItems([]core.ActivityItem{
		{Key: "c"}, {Key: "a"}, {Key: "b"},
	})
	if idx := m.SelectedIndex(); idx != 0 {
		t.Fatalf("after reorder SelectedIndex = %d, want 0", idx)
	}
	it, ok := m.SelectedItem()
	if !ok || it.Key != "c" {
		t.Fatalf("SelectedItem = %+v ok=%v, want key c", it, ok)
	}
}

func TestToggleUnreadOnly(t *testing.T) {
	m := New(nil, "")
	if m.UnreadOnly() {
		t.Fatal("UnreadOnly should default false")
	}
	if got := m.ToggleUnreadOnly(); !got || !m.UnreadOnly() {
		t.Fatalf("ToggleUnreadOnly = %v, want true", got)
	}
	if got := m.ToggleUnreadOnly(); got || m.UnreadOnly() {
		t.Fatalf("ToggleUnreadOnly = %v, want false", got)
	}
}

func TestUnreadCount(t *testing.T) {
	m := New(nil, "")
	m.SetItems([]core.ActivityItem{
		{Key: "a", IsUnread: true},
		{Key: "b"},
		{Key: "c", IsUnread: true},
	})
	if n := m.UnreadCount(); n != 2 {
		t.Errorf("UnreadCount = %d, want 2", n)
	}
}

// A card is always exactly cardContentLines lines, each one row and as
// wide as the list — the flat-list windowing / snap / click math depends
// on it. A body with embedded newlines must be collapsed, not split.
func TestRenderCardAlwaysThreeLines(t *testing.T) {
	cases := []struct {
		name string
		it   core.ActivityItem
		body core.ActivityMessage
	}{
		{"newline body", core.ActivityItem{Type: "at_user", ChannelID: "C1", TS: "1.1", AuthorID: "U1"},
			core.ActivityMessage{Text: "line one\nline two\nline three", UserID: "U1"}},
		{"empty body", core.ActivityItem{Type: "thread_v2", ChannelID: "C1", TS: "1.1"},
			core.ActivityMessage{}},
		{"wide CJK", core.ActivityItem{Type: "dm", ChannelID: "C1", TS: "1.1", AuthorID: "U1"},
			core.ActivityMessage{Text: "あいうえお　かきくけこ　さしすせそ　たちつてと", UserID: "U1"}},
		{"absent body key", core.ActivityItem{Type: "message_reaction", ChannelID: "CX", TS: "9.9", Reaction: "tada", AuthorID: "U1"},
			core.ActivityMessage{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := New(map[string]string{"U1": "alice"}, "")
			m.SetItems([]core.ActivityItem{c.it})
			m.SetBodies(map[string]core.ActivityMessage{
				core.ActivityMsgKey(c.it.ChannelID, c.it.TS): c.body,
			})
			for _, avatars := range []bool{false, true} {
				if avatars {
					m.SetAvatarFunc(func(string) string { return "AVAV\nAVAV" })
				}
				for n, l := range m.renderCard(c.it, 40, false) {
					if strings.Contains(l, "\n") {
						t.Fatalf("avatars=%v: card line %d contains a newline: %q", avatars, n+1, l)
					}
					if w := lipgloss.Width(l); w != 40 {
						t.Errorf("avatars=%v: card line %d is %d cells, want 40: %q", avatars, n+1, w, ansi.Strip(l))
					}
				}
			}
		})
	}
}

// renderLines lays cards out as three content lines each with one rule
// between adjacent cards (none after the last), as Slack's feed
// separates them. No line may contain a newline.
func TestRenderLinesSeparatesCards(t *testing.T) {
	m := New(nil, "")
	m.SetItems([]core.ActivityItem{
		{Type: "at_user", ChannelID: "C1", TS: "1.1", Key: "a"},
		{Type: "dm", ChannelID: "C2", TS: "2.2", Key: "b"},
		{Type: "thread_v2", ChannelID: "C3", TS: "3.3", Key: "c"},
	})
	lines := m.renderLines(0, 3*cardStride-1, 60)
	if want := 3*3 + 2; len(lines) != want {
		t.Fatalf("want %d lines (3 three-line cards + 2 separators), got %d", want, len(lines))
	}
	for i, l := range lines {
		if strings.Contains(l, "\n") {
			t.Fatalf("line %d contains a newline: %q", i, l)
		}
		isSeparator := i == 3 || i == 7
		if rule := strings.Contains(ansi.Strip(l), "───"); rule != isSeparator {
			t.Errorf("line %d rule=%v, want %v: %q", i, rule, isSeparator, ansi.Strip(l))
		}
		if w := lipgloss.Width(l); w != 60 {
			t.Errorf("line %d is %d cells, want 60", i, w)
		}
	}
}

// The card reads as Slack's does: who and when, what they said, where.
func TestRenderCard_LayoutIsAuthorBodyThenWhere(t *testing.T) {
	m := New(map[string]string{"U1": "alice"}, "")
	m.SetChannelNames(map[string]string{"C1": "general"})
	it := core.ActivityItem{Type: "thread_v2", ChannelID: "C1", TS: "1.1", AuthorID: "U1", IsUnread: true, FeedTS: "1700000000.0"}
	m.SetItems([]core.ActivityItem{it})
	m.SetBodies(map[string]core.ActivityMessage{core.ActivityMsgKey("C1", "1.1"): {Text: "the reply", UserID: "U1"}})

	card := m.renderCard(it, 60, false)
	plain := [cardContentLines]string{}
	for i, l := range card {
		plain[i] = strings.TrimRight(strings.TrimPrefix(ansi.Strip(l), "▌"), " ")
	}
	if !strings.HasPrefix(plain[0], "⚑ alice  ") || !strings.HasSuffix(plain[0], "●") || !strings.Contains(plain[0], " days") {
		t.Errorf("line 1 = %q, want glyph, author, age and the unread dot", plain[0])
	}
	if plain[1] != "  the reply" {
		t.Errorf("line 2 = %q, want the body under the author", plain[1])
	}
	if plain[2] != "  Thread in #general" {
		t.Errorf("line 3 = %q, want where it happened under the body", plain[2])
	}
	if !strings.Contains(card[0], "\x1b[1") {
		t.Errorf("unread author is not bold: %q", card[0])
	}
	it.IsUnread = false
	read := m.renderCard(it, 60, false)[0]
	if i := strings.Index(read, "alice"); i < 0 || strings.Contains(read[:i], "\x1b[1;") || strings.Contains(read[:i], "\x1b[1m") {
		t.Errorf("read author is bold, or missing: %q", read)
	}
}

// kittyAvatar is a 4x2 kitty placement, as the avatar cache hands one
// out, whose image ID ends in id.
func kittyAvatar(id int) string {
	row := func(mark rune) string {
		var b strings.Builder
		b.WriteString("\x1b[38;2;0;1;" + string(rune('0'+id)) + "m")
		for _, col := range []rune{'̅', '̍', '̎', '̐'} {
			b.WriteRune('\U0010EEEE')
			b.WriteRune(mark)
			b.WriteRune(col)
		}
		return b.String() + "\x1b[39m"
	}
	return row('̅') + "\n" + row('̍')
}

// With avatars, the author's sits beside lines 1 and 2, and a DM card
// shows the other person's even when you wrote last.
func TestRenderCard_AvatarBesideTheCard(t *testing.T) {
	m := New(map[string]string{"U1": "alice", "USELF": "me"}, "USELF")
	m.SetChannelNames(map[string]string{"D1": "alice", "C1": "general"})
	m.SetChannelTypes(map[string]string{"D1": "dm"})
	m.SetDMPeers(map[string]string{"D1": "U1"})
	asked := map[string]int{}
	m.SetAvatarFunc(func(userID string) string {
		asked[userID]++
		if userID == "U1" {
			return kittyAvatar(1)
		}
		return ""
	})
	dm := core.ActivityItem{Type: "dm", ChannelID: "D1", TS: "1.1"}
	reminder := core.ActivityItem{Type: "saved_reminder", Key: "r"}
	m.SetItems([]core.ActivityItem{dm, reminder})
	m.SetBodies(map[string]core.ActivityMessage{core.ActivityMsgKey("D1", "1.1"): {Text: "thanks", UserID: "USELF"}})

	top, bottom, _ := strings.Cut(kittyAvatar(1), "\n")
	for _, selected := range []bool{false, true} {
		card := m.renderCard(dm, 60, selected)
		if !strings.Contains(card[0], top) || !strings.Contains(card[1], bottom) {
			t.Fatalf("selected=%v: the other person's avatar is not intact beside lines 1 and 2", selected)
		}
		if strings.Contains(card[2], "\U0010EEEE") {
			t.Errorf("selected=%v: avatar ran into line 3", selected)
		}
		for n, l := range card {
			if w := lipgloss.Width(l); w != 60 {
				t.Errorf("selected=%v: line %d is %d cells beside a kitty avatar, want 60", selected, n+1, w)
			}
		}
		for n, want := range []string{"alice", "You: thanks", "Direct message"} {
			if got := ansi.Strip(card[n]); !strings.Contains(got, strings.Repeat(" ", avatarGutter-4)+want) {
				t.Errorf("selected=%v: line %d = %q, want %q after the avatar's gutter", selected, n+1, got, want)
			}
		}
	}
	if asked["USELF"] != 0 {
		t.Error("a DM card asked for your own avatar")
	}
	if got := strings.TrimPrefix(ansi.Strip(m.renderCard(reminder, 60, false)[0]), "▌"); !strings.HasPrefix(got, " ◷   Reminder") {
		t.Errorf("card with no avatar = %q, want its glyph in the avatar's place", got)
	}

	v := m.Version()
	m.HandleAvatarReady("U9")
	if m.Version() == v {
		t.Error("an avatar loading did not redraw the list")
	}
}

func TestView_HeadingCountsUnreadAndNamesTheFilter(t *testing.T) {
	m := New(nil, "")
	m.SetItems([]core.ActivityItem{{Key: "a", Type: "dm", IsUnread: true}, {Key: "b", Type: "dm"}})
	first := func() string { return strings.TrimRight(ansi.Strip(strings.SplitN(m.View(12, 50), "\n", 2)[0]), " ") }
	if got := first(); got != " Activity  ·  1 unread" {
		t.Errorf("heading = %q", got)
	}
	m.ToggleUnreadOnly()
	if got := first(); got != " Activity  ·  1 unread  ·  unreads only" {
		t.Errorf("heading with the filter on = %q", got)
	}
	m.SetItems(nil)
	if out := ansi.Strip(m.View(12, 50)); !strings.HasPrefix(out, " Activity  ·  unreads only") || !strings.Contains(out, "no unread activity") {
		t.Errorf("empty list lost its heading or its note:\n%s", out)
	}
	for _, h := range []int{1, 2, 3, 12} {
		if got := len(strings.Split(m.View(h, 50), "\n")); got != h {
			t.Errorf("View(%d) returned %d rows", h, got)
		}
	}
}

var sgrFgRe = regexp.MustCompile(`\x1b\[[0-9;]*?38;2;(\d+;\d+;\d+)[0-9;]*m`)

// fgOf returns the "r;g;b" truecolor foreground a style emits.
func fgOf(t *testing.T, c color.Color) string {
	t.Helper()
	m := sgrFgRe.FindStringSubmatch(lipgloss.NewStyle().Foreground(c).Render("x"))
	if m == nil {
		t.Fatalf("style for %v emitted no truecolor foreground", c)
	}
	return m[1]
}

// fgBefore returns the last truecolor foreground set before word in line.
func fgBefore(t *testing.T, line, word string) string {
	t.Helper()
	i := strings.Index(line, word)
	if i < 0 {
		t.Fatalf("%q not found in %q", word, ansi.Strip(line))
	}
	all := sgrFgRe.FindAllStringSubmatch(line[:i], -1)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1][1]
}

// The preview is the message itself, rendered like everywhere else in
// slk: emoji shortcodes become emoji, channel references resolve, and the
// text is in the normal text colour rather than the muted metadata one.
func TestRenderCard_PreviewRendersSlackMarkup(t *testing.T) {
	m := New(map[string]string{"U1": "alice"}, "")
	m.SetChannelNames(map[string]string{"C1": "general", "C9": "hiring"})
	it := core.ActivityItem{Type: "at_user", ChannelID: "C1", TS: "1.1", AuthorID: "U1"}
	m.SetItems([]core.ActivityItem{it})
	m.SetBodies(map[string]core.ActivityMessage{
		core.ActivityMsgKey("C1", "1.1"): {Text: ":wave: hello see <#C9>", UserID: "U1"},
	})

	l2 := m.renderCard(it, 80, false)[1]
	plain := ansi.Strip(l2)
	for _, want := range []string{"👋", "hello", "#hiring"} {
		if !strings.Contains(plain, want) {
			t.Errorf("preview missing %q: %q", want, plain)
		}
	}
	for _, raw := range []string{":wave:", "<#C9"} {
		if strings.Contains(plain, raw) {
			t.Errorf("preview shows raw markup %q: %q", raw, plain)
		}
	}
	if text, muted := fgOf(t, styles.TextPrimary), fgOf(t, styles.TextMuted); text == muted {
		t.Fatalf("theme's text and muted colours are equal (%s); test cannot tell them apart", text)
	} else if got := fgBefore(t, l2, "hello"); got != text {
		t.Errorf("preview text colour = %s, want the text colour %s (muted is %s)", got, text, muted)
	}
}

var sgrBgRe = regexp.MustCompile(`\x1b\[[0-9;]*?48;2;(\d+;\d+;\d+)[0-9;]*m`)

// The renderer restores the theme background after every styled span.
// On the selected card that must be the selection tint instead, or the
// text after a link or mention loses the highlight.
func TestRenderCard_SelectedPreviewKeepsSelectionTint(t *testing.T) {
	m := New(nil, "")
	m.SetChannelNames(map[string]string{"C9": "hiring"})
	it := core.ActivityItem{Type: "at_user", ChannelID: "C1", TS: "1.1"}
	m.SetItems([]core.ActivityItem{it})
	m.SetBodies(map[string]core.ActivityMessage{
		core.ActivityMsgKey("C1", "1.1"): {Text: "see <#C9> after"},
	})

	l2 := m.renderCard(it, 60, true)[1]
	i := strings.Index(l2, "after")
	if i < 0 {
		t.Fatalf("preview missing text: %q", ansi.Strip(l2))
	}
	bgs := sgrBgRe.FindAllStringSubmatch(l2[:i], -1)
	if len(bgs) == 0 {
		t.Fatal("no background set before the text")
	}
	tint := sgrBgRe.FindStringSubmatch(lipgloss.NewStyle().Background(styles.SelectionTintColor(false)).Render("x"))
	if got := bgs[len(bgs)-1][1]; got != tint[1] {
		t.Errorf("background behind text after a channel link = %s, want the selection tint %s", got, tint[1])
	}
}

// bgPerCell walks line's escape sequences and returns, for each printable
// rune, the truecolor background in effect ("" when none is set, i.e. the
// terminal default). OSC sequences (hyperlinks) are skipped.
func bgPerCell(line string) []string {
	var out []string
	bg := ""
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "\x1b]") {
			end := strings.Index(line[i:], "\x1b\\")
			if end < 0 {
				break
			}
			i += end + 2
			continue
		}
		if strings.HasPrefix(line[i:], "\x1b[") {
			end := strings.IndexByte(line[i:], 'm')
			params := strings.Split(line[i+2:i+end], ";")
			for j := 0; j < len(params); j++ {
				switch params[j] {
				case "", "0":
					bg = ""
				case "38", "48":
					if j+1 < len(params) && params[j+1] == "2" && j+4 < len(params) {
						if params[j] == "48" {
							bg = strings.Join(params[j+2:j+5], ";")
						}
						j += 4
					} else if j+1 < len(params) && params[j+1] == "5" {
						j += 2
					}
				}
			}
			i += end + 1
			continue
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		out = append(out, bg)
		i += size
	}
	return out
}

// Every cell of a card carries the row background (the selection tint
// when selected, the theme background otherwise), including
// text that follows a styled span (the bold author, a muted "You:", a
// channel link). A span's reset otherwise drops the row back to the
// terminal's default background.
func TestRenderCard_CardBackgroundIsUnbroken(t *testing.T) {
	m := New(map[string]string{"U2": "alice"}, "U1")
	m.SetChannelNames(map[string]string{"D1": "Alex", "C1": "general", "C9": "hiring"})
	m.SetChannelTypes(map[string]string{"D1": "dm", "C1": "channel"})
	tint := sgrBgRe.FindStringSubmatch(lipgloss.NewStyle().Background(styles.SelectionTintColor(false)).Render("x"))[1]

	for _, c := range []struct {
		name string
		it   core.ActivityItem
		body core.ActivityMessage
	}{
		{"your DM", core.ActivityItem{Type: "dm", ChannelID: "D1", TS: "1.1", IsUnread: true, FeedTS: "1700000000.0"},
			core.ActivityMessage{Text: "thanks", UserID: "U1"}},
		{"mention with link", core.ActivityItem{Type: "at_user", ChannelID: "C1", TS: "1.1", AuthorID: "U2"},
			core.ActivityMessage{Text: "see <#C9> after :wave: *bold* end", UserID: "U2"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m.SetItems([]core.ActivityItem{c.it})
			m.SetBodies(map[string]core.ActivityMessage{core.ActivityMsgKey(c.it.ChannelID, c.it.TS): c.body})
			themeBg := sgrBgRe.FindStringSubmatch(lipgloss.NewStyle().Background(styles.Background).Render("x"))[1]
			for _, sel := range []struct {
				selected bool
				want     string
			}{{true, tint}, {false, themeBg}} {
				for n, line := range m.renderCard(c.it, 80, sel.selected) {
					for i, bg := range bgPerCell(line) {
						if bg != sel.want {
							t.Errorf("selected=%v line %d cell %d background = %q, want %s\n%q",
								sel.selected, n+1, i, bg, sel.want, ansi.Strip(line))
							break
						}
					}
				}
			}
		})
	}
}

// A reaction card says what was reacted with, rendered as an emoji, and
// to what, as Slack words it; line 3 then only names the conversation.
func TestRenderCard_ReactionRendersEmoji(t *testing.T) {
	m := New(map[string]string{"U1": "alice"}, "")
	m.SetChannelNames(map[string]string{"C1": "general"})
	it := core.ActivityItem{Type: "message_reaction", ChannelID: "C1", TS: "1.1", Reaction: "eyes", AuthorID: "U1"}
	m.SetItems([]core.ActivityItem{it})

	card := m.renderCard(it, 80, false)
	plain := ansi.Strip(card[1])
	if !strings.Contains(plain, "Reacted 👀") || strings.Contains(plain, ":eyes:") {
		t.Errorf("reaction preview = %q, want \"Reacted\", the 👀 emoji and no :eyes: shortcode", plain)
	}
	m.SetBodies(map[string]core.ActivityMessage{core.ActivityMsgKey("C1", "1.1"): {Text: "ship it"}})
	card = m.renderCard(it, 80, false)
	if plain := ansi.Strip(card[1]); !strings.Contains(plain, "Reacted 👀 to: ship it") {
		t.Errorf("reaction preview = %q, want it to quote the message", plain)
	}
	if where := strings.TrimSpace(strings.TrimPrefix(ansi.Strip(card[2]), "▌")); where != "#general" {
		t.Errorf("line 3 = %q, want just the conversation", where)
	}
}

func TestContextVerb(t *testing.T) {
	cases := map[string]string{
		"at_user": "Mention", "at_channel": "Mention", "at_everyone": "Mention",
		"at_user_group": "Mention", "keyword": "Mention", "list_user_mentioned": "Mention",
		"thread_v2": "Thread", "message_reaction": "Reacted",
		"dm": "Direct message", "bot_dm_bundle": "Direct message",
		"saved_reminder": "Reminder",
		"":               "", "something_new": "",
	}
	for typ, want := range cases {
		if got := contextVerb(typ); got != want {
			t.Errorf("contextVerb(%q) = %q, want %q", typ, got, want)
		}
	}
}

// A DM's context omits the channel; a mention's context includes "#<channel>".
func TestContextLabel(t *testing.T) {
	m := New(nil, "")
	m.SetChannelNames(map[string]string{"C1": "general"})
	dm := m.contextLabel(core.ActivityItem{Type: "dm", ChannelID: "C1"}, true)
	if strings.Contains(dm, "general") || strings.Contains(dm, "#") || !strings.Contains(dm, "Direct message") {
		t.Fatalf("DM context should say \"Direct message\" and omit the channel, got %q", dm)
	}
	mention := m.contextLabel(core.ActivityItem{Type: "at_user", ChannelID: "C1"}, true)
	if !strings.Contains(mention, "#general") {
		t.Fatalf("mention context should contain #general, got %q", mention)
	}
}

// The context names the conversation the way the rest of slk does: "#"
// only for public channels, "◆" for private ones, "●" for group DMs, and
// just "DM" for a 1:1 DM (the other person is already on the card).
// Without the verb it is the conversation alone.
func TestContextLabel_ByConversationType(t *testing.T) {
	m := New(nil, "")
	m.SetChannelNames(map[string]string{
		"C1": "general", "G1": "secret", "D1": "Drew Gilliam", "M1": "Pav, Grant Ammons, Tom Hudson", "A1": "okta",
	})
	m.SetChannelTypes(map[string]string{
		"C1": "channel", "G1": "private", "D1": "dm", "M1": "group_dm", "A1": "app",
	})
	cases := []struct {
		name, typ, ch, want, bare string
	}{
		{"public channel", "at_user", "C1", "Mention in #general", "#general"},
		{"private channel", "at_user", "G1", "Mention in ◆ secret", "◆ secret"},
		{"1:1 DM", "message_reaction", "D1", "Reacted in DM", "Direct message"},
		{"app", "thread_v2", "A1", "Thread in App", "App"},
		{"group DM", "at_user", "M1", "Mention in ● Pav, Grant Ammons, Tom Hudson", "● Pav, Grant Ammons, Tom Hudson"},
		{"unknown type falls back to #", "at_user", "CX", "Mention in #CX", "#CX"},
	}
	for _, c := range cases {
		it := core.ActivityItem{Type: c.typ, ChannelID: c.ch}
		if got := ansi.Strip(m.contextLabel(it, true)); got != c.want {
			t.Errorf("%s: contextLabel = %q, want %q", c.name, got, c.want)
		}
		if got := ansi.Strip(m.contextLabel(it, false)); got != c.bare {
			t.Errorf("%s: contextLabel without the verb = %q, want %q", c.name, got, c.bare)
		}
	}
}

// An item type slk does not recognise used to render as a blank card:
// no verb, and (with no item.message to take a channel from) no author
// and no body either -- issue #285. The label falls back to the type
// itself so the row says what it is.
func TestContextLabel_UnknownTypeIsNeverBlank(t *testing.T) {
	m := New(nil, "")
	m.SetChannelNames(map[string]string{"C1": "general"})
	cases := []struct {
		name, typ, ch, want string
	}{
		{"unrecognised type, no channel", "brand_new_type", "", "Brand new type"},
		{"unrecognised type in a channel", "something_new", "C1", "Something new in #general"},
		{"empty type", "", "", "Unknown activity"},
	}
	for _, c := range cases {
		got := ansi.Strip(m.contextLabel(core.ActivityItem{Type: c.typ, ChannelID: c.ch}, true))
		if got != c.want {
			t.Errorf("%s: contextLabel = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRenderCard_UnknownTypeWithNoRefIsLabelled(t *testing.T) {
	m := New(nil, "")
	line1 := m.renderCard(core.ActivityItem{Key: "k", Type: "brand_new_type", FeedTS: "1700000000.000000", IsUnread: true}, 80, false)[0]
	if got := ansi.Strip(line1); !strings.Contains(got, "Brand new type") {
		t.Errorf("card line 1 = %q, want it to name the item type", got)
	}
}

// A DM row is headed by the conversation (the other person), not by
// whoever sent the latest message; when that was you, the preview says
// so, as Slack does.
func TestRenderCard_DMHeadedByConversation(t *testing.T) {
	m := New(map[string]string{"U1": "Alex Lazar", "USELF": "Grant"}, "USELF")
	m.SetChannelNames(map[string]string{"D1": "Alex Lazar"})
	m.SetChannelTypes(map[string]string{"D1": "dm"})

	for _, c := range []struct {
		name, from, wantPrefix string
	}{
		{"latest from you", "USELF", "You: "},
		{"latest from them", "U1", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			it := core.ActivityItem{Type: "dm", ChannelID: "D1", TS: "1.1"}
			m.SetItems([]core.ActivityItem{it})
			m.SetBodies(map[string]core.ActivityMessage{
				core.ActivityMsgKey("D1", "1.1"): {Text: "thanks", UserID: c.from},
			})
			card := m.renderCard(it, 80, false)
			l1, l2 := card[0], card[1]
			head := ansi.Strip(l1)
			if !strings.Contains(head, "Alex Lazar") || strings.Contains(head, "me ") {
				t.Errorf("header = %q, want it headed by the conversation, not the sender", head)
			}
			if where := strings.TrimSpace(strings.TrimPrefix(ansi.Strip(card[2]), "▌")); where != "Direct message" {
				t.Errorf("line 3 = %q, want \"Direct message\"", where)
			}
			body := strings.TrimSpace(strings.TrimPrefix(ansi.Strip(l2), "▌"))
			if want := c.wantPrefix + "thanks"; !strings.HasPrefix(body, want) {
				t.Errorf("preview = %q, want it to start with %q", body, want)
			}
			if got, want := fgBefore(t, l2, "thanks"), fgOf(t, styles.TextPrimary); got != want {
				t.Errorf("message colour = %s, want the text colour %s", got, want)
			}
		})
	}
}

// thread_v2 / dm refs carry no author on the feed item; the card falls back
// to the hydrated message's user so line 1 still names someone.
func TestRenderCard_AuthorFallbackToBody(t *testing.T) {
	m := New(map[string]string{"U9": "alice"}, "")
	it := core.ActivityItem{Type: "thread_v2", ChannelID: "C1", TS: "1.1"} // AuthorID == ""
	m.SetItems([]core.ActivityItem{it})
	m.SetBodies(map[string]core.ActivityMessage{
		core.ActivityMsgKey("C1", "1.1"): {Text: "the reply", UserID: "U9"},
	})
	l1 := m.renderCard(it, 60, false)[0]
	if !strings.Contains(l1, "alice") {
		t.Fatalf("author should fall back to hydrated body user 'alice', got line1=%q", l1)
	}
}

// A click on any line of a card selects that card; a click on the
// heading, a separator or past the last card selects nothing.
func TestClickAtStride(t *testing.T) {
	m := New(nil, "")
	m.SetItems([]core.ActivityItem{
		{Type: "at_user", ChannelID: "C1", TS: "1.1", Key: "a"},
		{Type: "dm", ChannelID: "C2", TS: "2.2", Key: "b"},
		{Type: "thread_v2", ChannelID: "C3", TS: "3.3", Key: "c"},
	})
	// The heading and its blank row are rows 0,1. Card 0 occupies rows
	// 2-4; separator 5; card 1 rows 6-8; separator 9; card 2 rows 10-12.
	for _, tc := range []struct {
		rowY    int
		wantOK  bool
		wantSel int
	}{
		{0, false, 0}, {1, false, 0},
		{2, true, 0}, {4, true, 0},
		{5, false, 0},
		{6, true, 1}, {8, true, 1},
		{9, false, 0},
		{10, true, 2}, {12, true, 2},
		{13, false, 0}, {-1, false, 0},
	} {
		ok := m.ClickAt(tc.rowY)
		if ok != tc.wantOK {
			t.Fatalf("ClickAt(%d) ok=%v, want %v", tc.rowY, ok, tc.wantOK)
		}
		if ok && m.SelectedIndex() != tc.wantSel {
			t.Fatalf("ClickAt(%d) selected=%d, want %d", tc.rowY, m.SelectedIndex(), tc.wantSel)
		}
	}
}

// PatchUserName delivers a name resolved after SetUserNames. The map is
// usually the App's shared one, which already holds the name, so the
// version must bump on the user being referenced, not on a map change.
func TestPatchUserName_BumpsVersionOnlyWhenFeedReferencesUser(t *testing.T) {
	shared := map[string]string{}
	m := New(shared, "USELF")
	m.SetItems([]core.ActivityItem{
		{Key: "a", Type: "at_user", ChannelID: "C1", TS: "1.0", AuthorID: "U1"},
		{Key: "b", Type: "thread_v2", ChannelID: "C1", TS: "2.0"},
	})
	m.SetBodies(map[string]core.ActivityMessage{
		core.ActivityMsgKey("C1", "1.0"): {Text: "hi <@U3>", UserID: "U1"},
		core.ActivityMsgKey("C1", "2.0"): {Text: "reply", UserID: "U2"},
	})
	for _, c := range []struct {
		uid  string
		want bool
	}{{"U1", true}, {"U2", true}, {"U3", true}, {"U4", false}, {"", false}} {
		shared[c.uid] = "name-" + c.uid // the App writes the shared map first
		v0 := m.Version()
		m.PatchUserName(c.uid, "name-"+c.uid)
		if bumped := m.Version() != v0; bumped != c.want {
			t.Errorf("PatchUserName(%q) bumped=%v, want %v", c.uid, bumped, c.want)
		}
	}
	if !strings.Contains(m.View(20, 60), "name-U1") {
		t.Error("View does not show the patched name")
	}
}

// Without a shared map (SetUserNames never called) the name must still
// land in the model's own map.
func TestPatchUserName_RecordsNameInOwnMap(t *testing.T) {
	m := New(nil, "USELF")
	m.SetItems([]core.ActivityItem{{Key: "a", Type: "at_user", ChannelID: "C1", TS: "1.0", AuthorID: "U1"}})
	m.PatchUserName("U1", "alice")
	if !strings.Contains(m.View(20, 60), "alice") {
		t.Error("View does not show the patched name")
	}
}

// A Slack reminder has no message to preview, so its card names it and
// says where to read it instead of leaving line 2 blank (issue #285).
func TestRenderCard_SavedReminder(t *testing.T) {
	m := New(nil, "")
	card := m.renderCard(core.ActivityItem{
		Key: "k", Type: "saved_reminder", Subtype: "reminder",
		FeedTS: "1700000000.000000", IsUnread: true,
	}, 80, false)
	line1, line2 := ansi.Strip(card[0]), ansi.Strip(card[1])
	if !strings.Contains(line1, "◷ Reminder") || !strings.Contains(line1, "●") {
		t.Errorf("line 1 = %q, want the reminder glyph, label and unread dot", line1)
	}
	if !strings.Contains(line2, "Open Slack to see this reminder") {
		t.Errorf("line 2 = %q, want the open-in-Slack hint", line2)
	}
}

// SetItemRead changes one item's dot in place. It never removes the
// item, even with the unread-only filter on, so U can undo; and it
// reports a key that is gone (the feed was refetched meanwhile).
func TestSetItemRead(t *testing.T) {
	m := New(nil, "")
	m.ToggleUnreadOnly()
	m.SetItems([]core.ActivityItem{{Key: "a", IsUnread: true}, {Key: "b"}})
	v := m.Version()

	if !m.SetItemRead("a", true) {
		t.Fatal(`SetItemRead("a", true) = false, want true`)
	}
	if m.Items()[0].IsUnread || m.UnreadCount() != 0 {
		t.Errorf("item a still unread after SetItemRead(a, true): %+v", m.Items()[0])
	}
	if m.Version() == v {
		t.Error("SetItemRead did not bump the version, so the frame would not redraw")
	}
	if len(m.Items()) != 2 {
		t.Errorf("items = %d, want 2: marking read must not drop the item", len(m.Items()))
	}
	if !m.SetItemRead("b", false) || !m.Items()[1].IsUnread {
		t.Errorf("SetItemRead(b, false) did not mark b unread: %+v", m.Items()[1])
	}
	if m.SetItemRead("gone", true) {
		t.Error(`SetItemRead("gone", true) = true for a key not in the list`)
	}
}
