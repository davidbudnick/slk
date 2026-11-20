package sidebar

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/core"
	imgpkg "github.com/gammons/slk/internal/image"
)

// kittyIcon is a 2x1 kitty placement as the avatar cache builds one,
// with id in the low byte of its image ID.
func kittyIcon(id int) string {
	var b strings.Builder
	b.WriteString("\x1b[38;2;0;1;" + string(rune('0'+id)) + "m")
	for _, col := range []rune{'̅', '̍'} {
		b.WriteRune(imgpkg.PlaceholderRune)
		b.WriteRune('̅')
		b.WriteRune(col)
	}
	return b.String() + "\x1b[39m"
}

// avatarSidebar is a sidebar of every row type whose avatars are loaded
// for U1 and UAPP, and still loading for U2.
func avatarSidebar(badges map[string]core.AvatarBadge) Model {
	m := New([]ChannelItem{
		{ID: "D1", Name: "ann", Type: "dm", DMUserID: "U1", Presence: "active", Section: "Team"},
		{ID: "D2", Name: "bob", Type: "dm", DMUserID: "U2", Presence: "away", Section: "Team"},
		{ID: "DA", Name: "jira", Type: "app", DMUserID: "UAPP", Section: "Team"},
		{ID: "G1", Name: "carl, dee", Type: "group_dm", Section: "Team"},
		{ID: "C1", Name: "general", Type: "channel", Section: "Team"},
		{ID: "P1", Name: "secret", Type: "private", Section: "Team"},
	})
	m.SetAvatarFunc(func(userID string, badge core.AvatarBadge) string {
		if badges != nil {
			badges[userID] = badge
		}
		switch userID {
		case "U1":
			return kittyIcon(1)
		case "UAPP":
			return kittyIcon(2)
		}
		return ""
	})
	return m
}

// nameColumn is the cell at which name starts in row.
func nameColumn(t *testing.T, row, name string) int {
	t.Helper()
	i := strings.Index(row, name)
	if i < 0 {
		t.Fatalf("%q not in row %q", name, row)
	}
	return lipgloss.Width(row[:i])
}

func TestAvatars_ReplaceTheGlyphAndKeepNamesInOneColumn(t *testing.T) {
	badges := map[string]core.AvatarBadge{}
	m := avatarSidebar(badges)
	view := m.View(14, 30)

	ann := rowFor(t, view, "ann")
	if !strings.Contains(ann, kittyIcon(1)) || strings.Contains(ann, "●") {
		t.Errorf("loaded DM row must draw its avatar, not the presence dot: %q", ann)
	}
	if jira := rowFor(t, view, "jira"); !strings.Contains(jira, kittyIcon(2)) || strings.Contains(jira, "▣") {
		t.Errorf("loaded app row must draw its avatar, not the app glyph: %q", jira)
	}
	plain := func(row string) string { return ansiRe.ReplaceAllString(row, "") }
	for name, want := range map[string]string{"bob": "○  bob", "general": "#  general", "secret": "◆  secret"} {
		if row := plain(rowFor(t, view, name)); !strings.Contains(row, want) {
			t.Errorf("%s row = %q, want its glyph padded to the avatar's width (%q)", name, row, want)
		}
	}
	want := nameColumn(t, plain(ann), "ann")
	for _, name := range []string{"bob", "jira", "carl, dee", "general", "secret"} {
		row := rowFor(t, view, name)
		if got := nameColumn(t, plain(row), name); got != want {
			t.Errorf("%s starts at cell %d, want %d like the avatar rows", name, got, want)
		}
		if w := lipgloss.Width(row); w != 28 {
			t.Errorf("%s row is %d cells, want 28: %q", name, w, plain(row))
		}
	}

	if b := badges["U1"]; b.Color == nil || b.Hollow {
		t.Errorf("active peer's badge = %+v, want a filled dot", b)
	}
	if b := badges["U2"]; b.Color == nil || !b.Hollow {
		t.Errorf("away peer's badge = %+v, want a ring", b)
	}
	if b := badges["UAPP"]; b.Color != nil {
		t.Errorf("app's badge = %+v, want none", b)
	}
}

// Each row variant re-applies its own colours after the image, so the
// name is never drawn in the terminal's default colour.
func TestAvatars_NameKeepsItsRowColours(t *testing.T) {
	m := avatarSidebar(nil)
	for _, selected := range []bool{false, true} {
		if selected {
			m.SelectByID("D1")
		}
		row := rowFor(t, m.View(14, 30), "ann")
		after := row[strings.Index(row, kittyIcon(1))+len(kittyIcon(1)):]
		if !strings.HasPrefix(after, "\x1b[0m\x1b[") {
			t.Errorf("selected %v: no colours re-applied after the avatar: %q", selected, after)
		}
	}
}

func TestAvatars_OffLeavesTheRowsAlone(t *testing.T) {
	m := avatarSidebar(nil)
	before := m.Version()
	m.SetAvatarFunc(nil)
	if m.Version() == before {
		t.Error("turning avatars off did not mark the sidebar changed")
	}
	plain := ansiRe.ReplaceAllString(m.View(14, 30), "")
	for _, want := range []string{"● ann", "○ bob", "▣ jira", "# general", "◆ secret"} {
		if !strings.Contains(plain, want) {
			t.Errorf("without avatars the view lost %q:\n%s", want, plain)
		}
	}
}

func TestAvatars_ReadyRedrawsOnlyForARowsPeer(t *testing.T) {
	m := avatarSidebar(nil)
	m.View(14, 30)
	before := m.Version()
	m.HandleAvatarReady("USTRANGER")
	if m.Version() != before {
		t.Error("an avatar no row draws redrew the sidebar")
	}
	m.HandleAvatarReady("U2")
	if m.Version() == before {
		t.Error("a DM peer's avatar loading did not redraw the sidebar")
	}
	m.SetAvatarFunc(nil)
	before = m.Version()
	m.HandleAvatarReady("U2")
	if m.Version() != before {
		t.Error("an avatar loading redrew a sidebar that draws none")
	}
}
