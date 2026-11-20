package commandpicker

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/core"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// workspace is a list as client.appCommands gives one: Slack's own
// commands, an alias, and apps' commands, in no useful order.
func workspace() []core.SlashCommand {
	return []core.SlashCommand{
		{Name: "/soc-handover", Desc: "On-Demand Handover", Type: core.SlashCommandApp, AppName: "Ticket Taco"},
		{Name: "/shrug", Usage: "your text", Desc: "Appends a shrug to your message", Type: core.SlashCommandCore},
		{Name: "/hangout", Desc: "Start a Google Meet", Type: core.SlashCommandService, AppName: "Google Meet"},
		{Name: "/dm", AliasOf: "/msg", Type: core.SlashCommandCore},
		{Name: "/msg", Usage: "@user [your message]", Type: core.SlashCommandCore},
		{Name: "/halp", Usage: "[description of issue]", Desc: "Raise a request", Type: core.SlashCommandApp, AppName: "Atlassian Assist"},
		{Name: "/attach", Usage: "[path]", Desc: "Attach a file from your computer", Type: core.SlashCommandLocal},
		{Name: "/challenge", Desc: "Challenge a player", Type: core.SlashCommandApp, AppName: "Wotscore"},
		{Name: "not-a-command"},
	}
}

func opened(t *testing.T, list []core.SlashCommand) Model {
	t.Helper()
	m := New(func() []core.SlashCommand { return list }, WithWidth(60))
	if !m.Open() {
		t.Fatal("Open reported no commands")
	}
	return m
}

func names(list []core.SlashCommand) string {
	out := make([]string, len(list))
	for i, c := range list {
		out[i] = c.Name
	}
	return strings.Join(out, " ")
}

// An empty query lists slk's own commands, then Slack's, then apps',
// each group by name. An alias is not listed: it still runs when typed.
func TestOpen_ListsByKindThenName(t *testing.T) {
	m := opened(t, workspace())
	if got, want := names(m.Filtered()), "/attach /msg /shrug /challenge /halp /hangout /soc-handover"; got != want {
		t.Errorf("menu = %q; want %q", got, want)
	}
}

// Slack's menu finds "/ha" at the start of a name first, then inside
// one: /hangout before /soc-handover.
func TestSetQuery_PrefixThenInside(t *testing.T) {
	m := opened(t, workspace())
	m.SetQuery("ha")
	if got, want := names(m.Filtered()), "/halp /hangout /soc-handover"; got != want {
		t.Errorf("/ha = %q; want %q", got, want)
	}
	m.SetQuery("HA")
	if got := names(m.Filtered()); got != "/halp /hangout /soc-handover" {
		t.Errorf("/HA = %q; want the same as /ha", got)
	}
	m.SetQuery("zzz")
	if m.HasMatches() || m.View() != "" {
		t.Errorf("a query nothing matches still shows %q", names(m.Filtered()))
	}
	if !m.IsVisible() {
		t.Error("the menu closed on a query nothing matches; deleting a letter must bring the rows back")
	}
	m.SetQuery("sh")
	if got := names(m.Filtered()); got != "/shrug" {
		t.Errorf("/sh after a miss = %q; want /shrug", got)
	}
}

func TestOpen_StaysClosedWithNothingToShow(t *testing.T) {
	for name, source := range map[string]SourceFunc{
		"no source":    nil,
		"no commands":  func() []core.SlashCommand { return nil },
		"only aliases": func() []core.SlashCommand { return []core.SlashCommand{{Name: "/dm", AliasOf: "/msg"}} },
	} {
		m := New(source)
		if m.Open() || m.IsVisible() || m.HasMatches() {
			t.Errorf("%s: the menu opened", name)
		}
		if _, ok := m.Select(); ok {
			t.Errorf("%s: Select returned a command", name)
		}
	}
}

// The list is read again at each Open, so a workspace switch or a list
// that arrived late is picked up without the picker being told.
func TestOpen_ReadsTheSourceEachTime(t *testing.T) {
	list := []core.SlashCommand{{Name: "/one"}}
	m := New(func() []core.SlashCommand { return list })
	m.Open()
	list = []core.SlashCommand{{Name: "/two"}}
	m.Close()
	if m.IsVisible() {
		t.Fatal("Close left the menu open")
	}
	m.Open()
	if got := names(m.Filtered()); got != "/two" {
		t.Errorf("second Open = %q; want /two", got)
	}
}

func TestMoveAndSelect(t *testing.T) {
	m := opened(t, workspace())
	m.MoveUp()
	if c, _ := m.Select(); c.Name != "/attach" {
		t.Errorf("MoveUp at the top selected %q; want /attach", c.Name)
	}
	for range 20 {
		m.MoveDown()
	}
	if c, ok := m.Select(); !ok || c.Name != "/soc-handover" {
		t.Errorf("MoveDown past the end selected %q; want the last, /soc-handover", c.Name)
	}
	m.SetQuery("s")
	if c, _ := m.Select(); c.Name != "/shrug" {
		t.Errorf("a new query selected %q; want its first row, /shrug", c.Name)
	}
}

// Each command takes two lines: its name and usage, then where it comes
// from and what it does. Every line is exactly the popup's width, long
// text is cut, and a list longer than the window gets a scrollbar.
func TestView_TwoLinesPerCommand(t *testing.T) {
	long := workspace()
	long = append(long, core.SlashCommand{
		Name: "/h-very-long-app-command", Usage: "with a usage hint that runs well past the popup",
		Desc: "and a description that does too, so both lines have to be cut to fit", Type: core.SlashCommandApp, AppName: "Verbose",
	})
	m := opened(t, long)
	m.SetQuery("h")
	view := m.View()
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		if w := lipgloss.Width(l); w != 58 {
			t.Errorf("line %d is %d cells wide; want 58 (width-2):\n%s", i, w, ansiRe.ReplaceAllString(view, ""))
		}
	}
	plain := ansiRe.ReplaceAllString(view, "")
	for _, want := range []string{
		"▌ /h-very-long-app-command with a usage hint",
		"▌ App · Verbose · and a description that does too",
		"/halp [description of issue]",
		"App · Atlassian Assist · Raise a request",
		"Command · Google Meet · Start a Google Meet",
		"App · Ticket Taco · On-Demand Handover",
		"…",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("view lacks %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "/challenge") || strings.Contains(plain, "/shrug") {
		t.Errorf("a name with h inside a word is listed for /h:\n%s", plain)
	}

	if strings.Contains(plain, "█") {
		t.Errorf("a list that fits its window has a scrollbar:\n%s", plain)
	}
	m.SetQuery("")
	if got := len(m.Filtered()); got <= MaxVisible {
		t.Fatalf("fixture has %d commands; need more than MaxVisible to scroll", got)
	}
	plain = ansiRe.ReplaceAllString(m.View(), "")
	for _, want := range []string{"/attach [path]", "slk · Attach a file from your computer"} {
		if !strings.Contains(plain, want) {
			t.Errorf("view lacks %q:\n%s", want, plain)
		}
	}
	if got := strings.Count(plain, "\n") + 1; got != 2*MaxVisible {
		t.Errorf("scrolling menu is %d lines; want %d rows of two lines (the test's styles draw no border)", got, MaxVisible)
	}
	if !strings.Contains(plain, "█") {
		t.Errorf("a list longer than the window has no scrollbar:\n%s", plain)
	}
	if strings.Contains(plain, "/soc-handover") {
		t.Errorf("the window shows a row past MaxVisible:\n%s", plain)
	}
	for range 20 {
		m.MoveDown()
	}
	if plain = ansiRe.ReplaceAllString(m.View(), ""); !strings.Contains(plain, "▌ /soc-handover") {
		t.Errorf("the window did not follow the selection to the last row:\n%s", plain)
	}
}
