package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/config"
)

func saveCollapsedForTest(t *testing.T, initial string, names []string) (string, config.Config) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if initial != "" {
		if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	if err := saveWorkspaceCollapsedSections(path, "acme", "T04T4TH8W", "My Team", names); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("the saved config no longer loads: %v\n%s", err, out)
	}
	return string(out), cfg
}

func TestSaveWorkspaceCollapsedSections_AddsKey(t *testing.T) {
	out, cfg := saveCollapsedForTest(t,
		"# My Team\n[workspaces.acme]\nteam_id = \"T04T4TH8W\"\ntheme = \"nord\"\n",
		[]string{"Team", "Direct messages"})

	if !strings.Contains(out, `collapsed_sections = ["Team", "Direct messages"]`) {
		t.Errorf("collapsed_sections not written:\n%s", out)
	}
	if !strings.Contains(out, "# My Team") || !strings.Contains(out, `theme = "nord"`) {
		t.Errorf("comment or sibling field lost:\n%s", out)
	}
	got, saved := cfg.ResolveCollapsedSections("T04T4TH8W")
	if !saved || !slices.Equal(got, []string{"Team", "Direct messages"}) {
		t.Errorf("loaded %q, saved=%v", got, saved)
	}
}

func TestSaveWorkspaceCollapsedSections_UpdatesKey(t *testing.T) {
	out, cfg := saveCollapsedForTest(t,
		"[workspaces.acme]\nteam_id = \"T04T4TH8W\"\ncollapsed_sections = [\"Old\"]\ntheme = \"nord\"\n",
		[]string{"New"})

	if strings.Contains(out, "Old") {
		t.Errorf("old list still present:\n%s", out)
	}
	if strings.Count(out, "collapsed_sections") != 1 {
		t.Errorf("want exactly one collapsed_sections entry:\n%s", out)
	}
	if got, _ := cfg.ResolveCollapsedSections("T04T4TH8W"); !slices.Equal(got, []string{"New"}) {
		t.Errorf("loaded %q, want [New]", got)
	}
	if cfg.ResolveTheme("T04T4TH8W") != "nord" {
		t.Errorf("sibling field clobbered:\n%s", out)
	}
}

func TestSaveWorkspaceCollapsedSections_EmptyListIsSaved(t *testing.T) {
	out, cfg := saveCollapsedForTest(t,
		"[workspaces.acme]\nteam_id = \"T04T4TH8W\"\ncollapsed_sections = [\"Old\"]\n",
		[]string{})

	if !strings.Contains(out, "collapsed_sections = []") {
		t.Errorf("empty list not written:\n%s", out)
	}
	got, saved := cfg.ResolveCollapsedSections("T04T4TH8W")
	if !saved || len(got) != 0 {
		t.Errorf("loaded %q, saved=%v; want an empty saved list", got, saved)
	}
}

func TestSaveWorkspaceCollapsedSections_ReplacesMultiLineList(t *testing.T) {
	initial := "[workspaces.acme]\n" +
		"team_id = \"T04T4TH8W\"\n" +
		"collapsed_sections = [\n" +
		"  \"A ] tricky # name\", # with a comment ]\n" +
		"\n" +
		"  'B',\n" +
		"]\n" +
		"theme = \"nord\"\n"
	out, cfg := saveCollapsedForTest(t, initial, []string{"Team"})

	if strings.Contains(out, "tricky") || strings.Contains(out, "'B'") {
		t.Errorf("continuation lines of the old list survived:\n%s", out)
	}
	if got, _ := cfg.ResolveCollapsedSections("T04T4TH8W"); !slices.Equal(got, []string{"Team"}) {
		t.Errorf("loaded %q, want [Team]", got)
	}
	if cfg.ResolveTheme("T04T4TH8W") != "nord" {
		t.Errorf("the entry after the list was lost:\n%s", out)
	}
}

func TestSaveWorkspaceCollapsedSections_FindsKeyAfterBlankLine(t *testing.T) {
	out, cfg := saveCollapsedForTest(t,
		"[workspaces.acme]\nteam_id = \"T04T4TH8W\"\n\n# my sections\ncollapsed_sections = [\"Old\"]\n\n[sidebar]\nwidth = 40\n",
		[]string{"New"})

	if strings.Count(out, "collapsed_sections") != 1 {
		t.Errorf("a second collapsed_sections entry makes the file invalid:\n%s", out)
	}
	if !strings.Contains(out, "# my sections") {
		t.Errorf("comment lost:\n%s", out)
	}
	if got, _ := cfg.ResolveCollapsedSections("T04T4TH8W"); !slices.Equal(got, []string{"New"}) {
		t.Errorf("loaded %q, want [New]", got)
	}
	if cfg.Sidebar.Width != 40 {
		t.Errorf("the next table was disturbed:\n%s", out)
	}
}

func TestSaveWorkspaceCollapsedSections_LeavesSubTablesAndOtherWorkspaces(t *testing.T) {
	initial := "[workspaces.acme]\n" +
		"team_id = \"T04T4TH8W\"\n" +
		"\n" +
		"[[workspaces.acme.suppress]]\n" +
		"author = \"U1\"\n" +
		"\n" +
		"[workspaces.other]\n" +
		"team_id = \"T05U5UI9X\"\n" +
		"collapsed_sections = [\"Theirs\"]\n"
	out, cfg := saveCollapsedForTest(t, initial, []string{"Mine"})

	if got, _ := cfg.ResolveCollapsedSections("T04T4TH8W"); !slices.Equal(got, []string{"Mine"}) {
		t.Errorf("acme loaded %q, want [Mine]\n%s", got, out)
	}
	if got, _ := cfg.ResolveCollapsedSections("T05U5UI9X"); !slices.Equal(got, []string{"Theirs"}) {
		t.Errorf("other workspace loaded %q, want [Theirs]\n%s", got, out)
	}
	ws, _ := cfg.WorkspaceByTeamID("T04T4TH8W")
	if len(ws.Suppress) != 1 || ws.Suppress[0].Author != "U1" {
		t.Errorf("suppress block lost: %+v\n%s", ws.Suppress, out)
	}
}

func TestSaveWorkspaceCollapsedSections_CreatesFileAndBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	if err := saveWorkspaceCollapsedSections(path, "T04T4TH8W", "T04T4TH8W", "My Team", []string{"Team"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, _ := os.ReadFile(path)
	if !strings.Contains(string(out), "# My Team\n[workspaces.T04T4TH8W]\ncollapsed_sections = [\"Team\"]") {
		t.Errorf("new block not written as expected:\n%s", out)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, saved := cfg.ResolveCollapsedSections("T04T4TH8W"); !saved || !slices.Equal(got, []string{"Team"}) {
		t.Errorf("loaded %q, saved=%v", got, saved)
	}
}

func TestSaveWorkspaceCollapsedSections_EscapesNames(t *testing.T) {
	names := []string{`He said "hi"`, `back\slash`, "tab\there", "] # bracket"}
	_, cfg := saveCollapsedForTest(t, "[workspaces.acme]\nteam_id = \"T04T4TH8W\"\n", names)

	if got, _ := cfg.ResolveCollapsedSections("T04T4TH8W"); !slices.Equal(got, names) {
		t.Errorf("loaded %q, want %q", got, names)
	}
}

func TestSaveWorkspaceEntry_BlankLineDoesNotHideAnEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	initial := "[workspaces.acme]\nteam_id = \"T04T4TH8W\"\n\nsidebar_width = 30\nversion_ts = \"1\"\ntheme = \"nord\"\n"
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := saveWorkspaceWidth(path, "acme", "T04T4TH8W", "My Team", 44); err != nil {
		t.Fatalf("save width: %v", err)
	}
	if err := saveWorkspaceVersionTS(path, "acme", "T04T4TH8W", "My Team", "2"); err != nil {
		t.Fatalf("save version_ts: %v", err)
	}
	if err := saveWorkspaceTheme(path, "acme", "T04T4TH8W", "My Team", "dracula"); err != nil {
		t.Fatalf("save theme: %v", err)
	}
	out, _ := os.ReadFile(path)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("a duplicate key made the config unreadable: %v\n%s", err, out)
	}
	ws, _ := cfg.WorkspaceByTeamID("T04T4TH8W")
	if ws.SidebarWidth != 44 || ws.VersionTS != "2" || ws.Theme != "dracula" {
		t.Errorf("got width=%d version_ts=%q theme=%q\n%s", ws.SidebarWidth, ws.VersionTS, ws.Theme, out)
	}
}

func TestArrayDepthDelta(t *testing.T) {
	cases := []struct {
		line string
		want int
	}{
		{`collapsed_sections = ["a", "b"]`, 0},
		{`collapsed_sections = [`, 1},
		{`  "a ] b", 'c ] d',`, 0},
		{`]`, -1},
		{`  "x", # trailing ]`, 0},
		{`x = [["a"], [`, 2},
		{`  "quote \" ] still inside"`, 0},
		{`  'literal \' ]`, -1},
	}
	for _, tc := range cases {
		if got := arrayDepthDelta(tc.line); got != tc.want {
			t.Errorf("arrayDepthDelta(%q) = %d, want %d", tc.line, got, tc.want)
		}
	}
}

func TestWorkspaceConfigStore_SaveCollapsedSections(t *testing.T) {
	store := newWorkspaceConfigStore(config.Config{Workspaces: map[string]config.Workspace{
		"acme": {TeamID: "T04T4TH8W"},
	}})
	if _, saved := store.Snapshot().ResolveCollapsedSections("T04T4TH8W"); saved {
		t.Fatal("precondition: nothing saved yet")
	}

	names := []string{"Team"}
	if key := store.SaveCollapsedSections("T04T4TH8W", names); key != "acme" {
		t.Errorf("tomlKey = %q, want acme", key)
	}
	names[0] = "changed"
	if got, saved := store.Snapshot().ResolveCollapsedSections("T04T4TH8W"); !saved || !slices.Equal(got, []string{"Team"}) {
		t.Errorf("snapshot = %q, saved=%v; want [Team], true", got, saved)
	}

	store.SaveCollapsedSections("T04T4TH8W", nil)
	if got, saved := store.Snapshot().ResolveCollapsedSections("T04T4TH8W"); !saved || len(got) != 0 {
		t.Errorf("after saving none: %q, saved=%v; want an empty saved list", got, saved)
	}

	if key := store.SaveCollapsedSections("T99NEW", []string{"X"}); key != "T99NEW" {
		t.Errorf("tomlKey for an unconfigured workspace = %q, want the team ID", key)
	}
}
