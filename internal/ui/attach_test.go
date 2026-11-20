package ui

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
)

func TestShellWords(t *testing.T) {
	tests := []struct {
		in   string
		want []string
		ok   bool
	}{
		{`/tmp/a.png`, []string{"/tmp/a.png"}, true},
		{`/tmp/Screen\ Shot\ 1.png`, []string{"/tmp/Screen Shot 1.png"}, true},
		{`/tmp/a.png /tmp/b\ c.png`, []string{"/tmp/a.png", "/tmp/b c.png"}, true},
		{`'/tmp/it is.png' "/tmp/so \"is\" this.png"`, []string{"/tmp/it is.png", `/tmp/so "is" this.png`}, true},
		{`'/tmp/back\slash.png'`, []string{`/tmp/back\slash.png`}, true},
		{"  /tmp/a.png \t ", []string{"/tmp/a.png"}, true},
		{`/tmp/at 4.31.15` + " " + `PM.mov`, []string{"/tmp/at", "4.31.15 PM.mov"}, true},
		{``, nil, true},
		{`'/tmp/open`, nil, false},
		{`/tmp/trailing\`, nil, false},
		{"/tmp/a.png\n/tmp/b.png", nil, false},
	}
	for _, tt := range tests {
		got, ok := shellWords(tt.in)
		if ok != tt.ok || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("shellWords(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

// A terminal types dropped files as a shell would: spaces escaped (the
// narrow space macOS puts in a screenshot's name is not one), or each
// path quoted, several separated by spaces. A copied path arrives as
// it is, spaces and all. Both readings are offered; the caller keeps
// the first whose files exist.
func TestPathReadings(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want [][]string
	}{
		{"a plain path", "/tmp/a.png", [][]string{{"/tmp/a.png"}}},
		{"a copied path with spaces", "/tmp/My File.png", [][]string{{"/tmp/My File.png"}}},
		{"a dropped path, spaces escaped", `/tmp/Screen\ Recording\ at\ 4.31.15` + " " + `PM.mov`,
			[][]string{{`/tmp/Screen\ Recording\ at\ 4.31.15` + " " + `PM.mov`}, {"/tmp/Screen Recording at 4.31.15 PM.mov"}}},
		{"two dropped paths", `/tmp/a.png /tmp/b\ c.png`, [][]string{{"/tmp/a.png /tmp/b\\ c.png"}, {"/tmp/a.png", "/tmp/b c.png"}}},
		{"a quoted path", `'/tmp/it is.png'`, [][]string{{"/tmp/it is.png"}}},
		{"a file URL", "file:///tmp/My%20File.png", [][]string{{"/tmp/My File.png"}}},
		{"a file URL on another host", "file://server/tmp/a.png", nil},
		{"a sentence", "see /tmp/a.png for details", nil},
		{"a bare name", "notes.txt", nil},
		{"a command", "/giphy cats", [][]string{{"/giphy cats"}}},
		{"nothing", "   ", nil},
	}
	for _, tt := range tests {
		if got := pathReadings(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: pathReadings(%q) = %q; want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

// attachApp is an app on channel C1 over real files: dir holds
// "Screen Shot 1.png" and "b.txt" (a few bytes each), "empty.bin" and a
// directory "sub". chosen is what the file chooser returns.
func attachApp(t *testing.T, chosen func() ([]string, error)) (a *App, dir string) {
	t.Helper()
	dir = t.TempDir()
	for name, body := range map[string]string{"Screen Shot 1.png": "png", "b.txt": "text", "empty.bin": ""} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	a = newTestAppWithMessages(t)
	a.activeChannelID = "C1"
	a.focusedPanel = PanelMessages
	a.SetDesktopService(core.NewDesktopService(core.DesktopServiceFuncs{Stat: os.Stat, ChooseFiles: chosen}))
	return a, dir
}

func attachedNames(a *App) string {
	var names []string
	for _, att := range a.compose.Attachments() {
		names = append(names, att.Filename)
	}
	return strings.Join(names, ",")
}

func shellEscape(path string) string { return strings.ReplaceAll(path, " ", `\ `) }

// Files dropped on the terminal attach, whatever mode slk is in: the
// drop arrives as a paste of their paths, escaped as a shell would. In
// normal mode the composer takes the keyboard, as Slack's does when a
// file is dropped on its window.
func TestDrop_AttachesFilesInAnyMode(t *testing.T) {
	a, dir := attachApp(t, nil)
	shot := shellEscape(filepath.Join(dir, "Screen Shot 1.png"))

	a.Update(tea.PasteMsg{Content: shot})
	if got := attachedNames(a); got != "Screen Shot 1.png" {
		t.Fatalf("after a drop in normal mode, attachments = %q; want the screenshot", got)
	}
	if a.mode != ModeInsert || a.compose.Value() != "" {
		t.Errorf("after a drop in normal mode: mode=%v text=%q; want insert mode and no pasted text", a.mode, a.compose.Value())
	}
	if !strings.Contains(a.statusbar.View(a.width), "Attached: Screen Shot 1.png") {
		t.Errorf("no toast names the attached file: %q", a.statusbar.View(a.width))
	}

	a.Update(tea.PasteMsg{Content: shot + " " + filepath.Join(dir, "b.txt") + " " + filepath.Join(dir, "empty.bin")})
	if got := attachedNames(a); got != "Screen Shot 1.png,Screen Shot 1.png,b.txt" {
		t.Errorf("after dropping three files in insert mode, attachments = %q; want two more, the empty one left out", got)
	}
	if bar := a.statusbar.View(a.width); !strings.Contains(bar, "Attached 2 files; left out empty.bin (empty)") {
		t.Errorf("toast = %q; want the count and what was left out", bar)
	}
	if a.compose.Value() != "" {
		t.Errorf("the dropped paths were also pasted as text: %q", a.compose.Value())
	}
}

// Text that is not a file is still text in insert mode and still
// nothing in normal mode; a path to something that is not a regular
// file, or one of several that is missing, is not a drop.
func TestDrop_LeavesOrdinaryTextAlone(t *testing.T) {
	a, dir := attachApp(t, nil)
	for _, text := range []string{"hello pasted", "/giphy cats", filepath.Join(dir, "sub"), filepath.Join(dir, "missing.png")} {
		a.Update(tea.PasteMsg{Content: text})
		if a.mode != ModeNormal || len(a.compose.Attachments()) != 0 || a.compose.Value() != "" {
			t.Fatalf("pasting %q in normal mode: mode=%v attachments=%q text=%q; want it ignored",
				text, a.mode, attachedNames(a), a.compose.Value())
		}
	}

	a.SetMode(ModeInsert)
	_ = a.compose.Focus()
	mixed := filepath.Join(dir, "b.txt") + " " + filepath.Join(dir, "missing.png")
	a.Update(tea.PasteMsg{Content: mixed})
	if len(a.compose.Attachments()) != 0 || a.compose.Value() != mixed {
		t.Errorf("pasting an existing and a missing path: attachments=%q text=%q; want it kept as text", attachedNames(a), a.compose.Value())
	}

	b, _ := attachApp(t, nil)
	b.composeDisabled = true
	b.Update(tea.PasteMsg{Content: filepath.Join(dir, "b.txt")})
	if b.mode != ModeNormal || len(b.compose.Attachments()) != 0 {
		t.Error("a drop attached to a conversation that takes no messages")
	}
}

// /attach with a path attaches it and stays in the composer for a
// caption; a path with nothing at it is left to be corrected. It is
// the one command that can follow files already attached.
func TestAttachCommand_WithAPath(t *testing.T) {
	a, dir := attachApp(t, nil)
	a.SetMode(ModeInsert)
	_ = a.compose.Focus()

	a.compose.SetValue("/attach " + filepath.Join(dir, "missing.png"))
	a.Update(keyCode(tea.KeyEnter))
	if len(a.compose.Attachments()) != 0 || !strings.HasPrefix(a.compose.Value(), "/attach ") {
		t.Fatalf("a missing path: attachments=%q text=%q; want nothing attached and the text kept", attachedNames(a), a.compose.Value())
	}
	if !strings.Contains(a.statusbar.View(a.width), "No file at") {
		t.Errorf("no toast for the missing path: %q", a.statusbar.View(a.width))
	}

	for _, typed := range []string{
		"/attach " + filepath.Join(dir, "Screen Shot 1.png"),
		"/attach " + shellEscape(filepath.Join(dir, "Screen Shot 1.png")) + " " + filepath.Join(dir, "b.txt"),
	} {
		a.compose.SetValue(typed)
		a.Update(keyCode(tea.KeyEnter))
		if a.compose.Value() != "" || a.mode != ModeInsert {
			t.Errorf("after %q: text=%q mode=%v; want the command cleared and insert mode kept", typed, a.compose.Value(), a.mode)
		}
	}
	if got := attachedNames(a); got != "Screen Shot 1.png,Screen Shot 1.png,b.txt" {
		t.Errorf("attachments = %q; want one from the first command and two from the second", got)
	}
}

// /attach alone opens the system's file chooser and attaches what it
// returns to the composer that asked; a cancel attaches nothing, and a
// system with no chooser says how to give a path.
func TestAttachCommand_OpensTheChooser(t *testing.T) {
	var answer []string
	var failure error
	a, dir := attachApp(t, func() ([]string, error) { return answer, failure })
	a.SetMode(ModeInsert)
	_ = a.compose.Focus()
	run := func() {
		t.Helper()
		a.compose.SetValue("/attach")
		_, cmd := a.Update(keyCode(tea.KeyEnter))
		if a.compose.Value() != "" {
			t.Fatalf("/attach left %q in the composer", a.compose.Value())
		}
		for _, m := range drainBatch(cmd) {
			a.Update(m)
		}
	}

	answer = []string{filepath.Join(dir, "Screen Shot 1.png"), filepath.Join(dir, "b.txt")}
	run()
	if got := attachedNames(a); got != "Screen Shot 1.png,b.txt" {
		t.Fatalf("attachments = %q; want what the chooser returned", got)
	}

	answer = nil
	run()
	if got := attachedNames(a); got != "Screen Shot 1.png,b.txt" {
		t.Errorf("a cancelled chooser changed the attachments: %q", got)
	}

	failure = errors.ErrUnsupported
	run()
	if bar := a.statusbar.View(a.width); !strings.Contains(bar, "No file chooser here") {
		t.Errorf("toast = %q; want it to say there is no chooser", bar)
	}
}

// The "/" menu offers /attach first, beside the workspace's commands.
func TestSlashMenu_OffersAttach(t *testing.T) {
	a, _ := menuApp(t)
	list := a.slashCommandList()
	if len(list) != 3 || list[0] != attachMenuEntry {
		t.Fatalf("menu list = %+v; want /attach, then the workspace's two commands", list)
	}
	typeKeysIntoApp(a, "/att")
	if view := a.View().Content; !strings.Contains(view, "slk · Attach a file from your computer") {
		t.Errorf("the menu does not show /attach's row")
	}
}
