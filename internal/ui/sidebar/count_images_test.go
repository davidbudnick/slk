package sidebar

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/core"
	imgpkg "github.com/gammons/slk/internal/image"
)

// countPlacement is a kitty placement as the avatar cache builds one
// for c: a tile two cells wide, a pill one cell wider than its label.
func countPlacement(c core.CountImage) string {
	cells := len(c.Label) + 1
	if c.Tile {
		cells = iconCells
	}
	id := strings.ReplaceAll(c.Label, "+", "0")
	lead := "\x1b[38;2;0;2;" + id + "m"
	if c.Tile {
		lead = "\x1b[38;2;0;3;" + id + "m"
	}
	return lead + strings.Repeat(string(imgpkg.PlaceholderRune)+"̅", cells) + "\x1b[39m"
}

// countSidebar is avatarSidebar with counts drawn as images, a group DM
// of three others, and mentions on ann (3) and general (12). asked
// collects what the sidebar asked to have drawn.
func countSidebar(asked *[]core.CountImage) Model {
	m := avatarSidebar(nil)
	items := append([]ChannelItem(nil), m.items...)
	for i := range items {
		if items[i].ID == "G1" {
			items[i].GroupSize = 3
		}
	}
	m.SetItems(items)
	m.SetReadStateReader(func() map[string]cache.ReadState {
		return map[string]cache.ReadState{
			"D1": {HasUnread: true, MentionCount: 3},
			"C1": {HasUnread: true, MentionCount: 12},
		}
	})
	m.SetRightAlignBadges(true)
	m.SetCountFunc(func(c core.CountImage) string {
		if asked != nil {
			*asked = append(*asked, c)
		}
		return countPlacement(c)
	})
	return m
}

// Slack draws a group DM's icon as the number of people in it, on a
// square the size of an avatar, so it lines up with the avatars around
// it where a one-cell dot does not.
func TestCountImages_GroupDMIsATileInTheAvatarColumn(t *testing.T) {
	var asked []core.CountImage
	m := countSidebar(&asked)
	view := m.View(14, 30)
	plain := func(row string) string { return ansiRe.ReplaceAllString(row, "") }

	group := rowFor(t, view, "carl, dee")
	tile := countPlacement(core.CountImage{Label: "3", Tile: true})
	if !strings.Contains(group, tile) || strings.Contains(group, "●") {
		t.Fatalf("group DM row must draw its size on a tile, not the dot: %q", group)
	}
	if got, want := nameColumn(t, plain(group), "carl, dee"), nameColumn(t, plain(rowFor(t, view, "ann")), "ann"); got != want {
		t.Errorf("group DM's name starts at cell %d, want %d like the avatar rows", got, want)
	}
	if w := lipgloss.Width(group); w != 28 {
		t.Errorf("group DM row is %d cells, want 28: %q", w, plain(group))
	}
	if after := group[strings.Index(group, tile)+len(tile):]; !strings.HasPrefix(after, "\x1b[0m\x1b[") {
		t.Errorf("no colours re-applied after the tile: %q", after)
	}
	var tiles []core.CountImage
	for _, c := range asked {
		if c.Tile {
			tiles = append(tiles, c)
		}
	}
	if len(tiles) == 0 || tiles[0].Label != "3" || tiles[0].Fill == nil || tiles[0].Text == nil {
		t.Errorf("tiles asked for = %+v, want the group's size in the sidebar's colours", tiles)
	}

	// A group DM of unknown size keeps its glyph.
	items := append([]ChannelItem(nil), m.items...)
	for i := range items {
		items[i].GroupSize = 0
	}
	m.SetItems(items)
	if row := plain(rowFor(t, m.View(14, 30), "carl, dee")); !strings.Contains(row, "●  carl, dee") {
		t.Errorf("group DM of unknown size = %q, want its padded glyph", row)
	}
}

// A badge drawn in cells fills them top to bottom, so the badges of
// adjacent rows touch. Slack's are pills with a gap between them.
func TestCountImages_MentionBadgeIsAPillAtTheRowsEnd(t *testing.T) {
	m := countSidebar(nil)
	m.SetThreadsUnreadCount(7)
	m.SetActivityUnreadCount(120)
	view := m.View(14, 30)
	plain := func(row string) string { return ansiRe.ReplaceAllString(row, "") }

	ends := map[string]int{}
	for name, label := range map[string]string{"ann": "3", "general": "12", "Threads": "7", "Activity": "99+"} {
		row := rowFor(t, view, name)
		pill := countPlacement(core.CountImage{Label: label})
		i := strings.Index(row, pill)
		if i < 0 {
			t.Fatalf("%s: row does not draw its count %s as a pill: %q", name, label, row)
		}
		if strings.Contains(plain(row), label) {
			t.Errorf("%s: row still draws %s in cells: %q", name, label, plain(row))
		}
		if w := lipgloss.Width(row); w != 28 {
			t.Errorf("%s: row is %d cells, want 28", name, w)
		}
		ends[name] = lipgloss.Width(row[:i+len(pill)])
	}
	for name, end := range ends {
		if end != ends["ann"] {
			t.Errorf("%s's pill ends at cell %d, ann's at %d; want one right edge", name, end, ends["ann"])
		}
	}

	m.ToggleCollapse("Team")
	header := rowFor(t, m.View(14, 30), "Team")
	if !strings.Contains(header, countPlacement(core.CountImage{Label: "15"})) {
		t.Errorf("collapsed header does not draw its mentions as a pill: %q", header)
	}
	if w := lipgloss.Width(header); w > 28 {
		t.Errorf("collapsed header is %d cells, want at most 28", w)
	}
}

func TestCountImages_OffRestoresTheThemesBadge(t *testing.T) {
	m := countSidebar(nil)
	before := m.Version()
	m.SetCountFunc(nil)
	if m.Version() == before {
		t.Error("turning count images off did not mark the sidebar changed")
	}
	plain := ansiRe.ReplaceAllString(m.View(14, 30), "")
	for _, want := range []string{" 3 ", " 12 ", "●  carl, dee"} {
		if !strings.Contains(plain, want) {
			t.Errorf("without count images the view lost %q:\n%s", want, plain)
		}
	}
}
