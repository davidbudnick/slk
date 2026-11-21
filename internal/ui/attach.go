package ui

import (
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ui/compose"
)

// attachCommand is slk's own slash command for attaching a file: Slack
// puts "Upload from your computer" on its composer's "+" menu.
const attachCommand = "/attach"

// attachMenuEntry is attachCommand's row in the "/" menu.
var attachMenuEntry = core.SlashCommand{
	Name:  attachCommand,
	Usage: "[path]",
	Desc:  "Attach a file from your computer",
	Type:  core.SlashCommandLocal,
}

// pathReadings returns the ways text can name files, most literal
// first: as one path, the way a copied path arrives, and as the words a
// terminal types for files dropped on it, with spaces escaped or each
// path quoted and several paths separated by spaces. Each reading is a
// list of cleaned paths; text that is neither has none.
func pathReadings(text string) [][]string {
	var out [][]string
	if p, ok := resolveFilePath(text); ok {
		out = append(out, []string{p})
	}
	words, ok := shellWords(strings.TrimSpace(text))
	if !ok || len(words) == 0 {
		return out
	}
	paths := make([]string, 0, len(words))
	for _, w := range words {
		p, ok := resolveFilePath(w)
		if !ok {
			return out
		}
		paths = append(paths, p)
	}
	if len(out) == 1 && len(paths) == 1 && paths[0] == out[0][0] {
		return out
	}
	return append(out, paths)
}

// shellWords splits s as a shell would: a backslash escapes the next
// character, quotes group, and unquoted spaces separate. It reports
// false for text no shell would have typed as one line of words: a
// newline, an open quote, a trailing backslash.
func shellWords(s string) ([]string, bool) {
	var words []string
	var cur strings.Builder
	inWord := false
	quote := rune(0)
	escaped := false
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r':
			return nil, false
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped, inWord = true, true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if escaped || quote != 0 {
		return nil, false
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, true
}

// attachNamed attaches the files text names to target, by the first of
// its readings whose every path is a regular file. It reports false
// when text names no files.
func (a *App) attachNamed(target *compose.Model, text string) (tea.Cmd, bool) {
	for _, paths := range pathReadings(text) {
		if cmd, ok := a.attachPaths(target, paths); ok {
			return cmd, true
		}
	}
	return nil, false
}

// attachPaths attaches the files at paths to target when every path is
// a regular file, and reports false otherwise so the text can be read
// another way. A file over the size limit or with nothing in it is left
// out, and the toast says so.
func (a *App) attachPaths(target *compose.Model, paths []string) (tea.Cmd, bool) {
	sizes := make([]int64, len(paths))
	for i, p := range paths {
		info, err := a.desktop.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			return nil, false
		}
		sizes[i] = info.Size()
	}
	if len(paths) == 0 {
		return nil, false
	}
	var attached, refused []string
	for i, p := range paths {
		name := filepath.Base(p)
		switch {
		case sizes[i] > maxAttachmentSize:
			refused = append(refused, name+" (over 10 MB)")
		case sizes[i] == 0:
			refused = append(refused, name+" (empty)")
		default:
			target.AddAttachment(core.PendingAttachment{
				Filename: name,
				Path:     p,
				Mime:     mime.TypeByExtension(filepath.Ext(p)),
				Size:     sizes[i],
			})
			attached = append(attached, fmt.Sprintf("%s (%s)", name, humanSize(sizes[i])))
		}
	}
	if len(paths) == 1 {
		switch {
		case sizes[0] > maxAttachmentSize:
			return toastWithClear(a, "File too large (>10 MB limit)", 3*time.Second), true
		case sizes[0] == 0:
			return toastWithClear(a, "Empty file", 2*time.Second), true
		}
		return toastWithClear(a, "Attached: "+attached[0], 2*time.Second), true
	}
	text := fmt.Sprintf("Attached %d files", len(attached))
	if len(refused) > 0 {
		text += "; left out " + strings.Join(refused, ", ")
	}
	return toastWithClear(a, text, 4*time.Second), true
}

// attachDropped handles text pasted outside insert mode. Files dropped
// on the terminal arrive that way, as their paths: when the text names
// files they are attached to the open conversation's composer, which
// takes the keyboard, as dropping files on Slack's window does. Any
// other text is ignored, as it always was.
func (a *App) attachDropped(content string) tea.Cmd {
	if a.mode != ModeNormal {
		return nil
	}
	toThread := a.focusedPanel == PanelThread || (a.view == ViewThreads && a.threadVisible) || a.threadDrawnAlone()
	target := &a.compose
	if toThread {
		target = &a.threadCompose
	}
	if !toThread && (a.composeDisabled || a.activeChannelID == "") {
		return nil
	}
	if a.compose.Uploading() || a.threadCompose.Uploading() {
		return toastWithClear(a, "Upload in progress", 2*time.Second)
	}
	if cmd, ok := a.attachNamed(target, content); ok {
		return tea.Batch(a.beginInsertMode(toThread), cmd)
	}
	return nil
}

// filesChosenMsg is the system file chooser's answer to /attach: the
// picked paths, none when it was cancelled. thread says which composer
// asked.
type filesChosenMsg struct {
	thread bool
	paths  []string
	err    error
}

// runAttachCommand is /attach typed in composer c. With a path (or
// several, as a shell would write them) it attaches those files; alone
// it opens the system's file chooser. The command's text is cleared
// and c stays in insert mode with what was attached, ready for a
// caption. A path that names no file stays in the composer to be
// corrected.
func (a *App) runAttachCommand(panel Panel, c *compose.Model, raw string) tea.Cmd {
	args := strings.TrimSpace(raw[len(attachCommand):])
	if args == "" {
		c.SetValue("")
		desktop, thread := a.desktop, panel == PanelThread
		return func() tea.Msg {
			paths, err := desktop.ChooseFiles()
			return filesChosenMsg{thread: thread, paths: paths, err: err}
		}
	}
	if cmd, ok := a.attachNamed(c, args); ok {
		c.SetValue("")
		return cmd
	}
	return toastWithClear(a, "No file at "+args, 3*time.Second)
}

// reduceFilesChosen attaches what the file chooser returned to the
// composer that asked.
func (a *App) reduceFilesChosen(m filesChosenMsg) tea.Cmd {
	switch {
	case errors.Is(m.err, errors.ErrUnsupported):
		return toastWithClear(a, "No file chooser here; give a path: /attach ~/file", 4*time.Second)
	case m.err != nil:
		debuglog.General("file chooser: %v", m.err)
		return toastWithClear(a, "The file chooser failed; give a path: /attach ~/file", 4*time.Second)
	case len(m.paths) == 0:
		return nil
	}
	if a.compose.Uploading() || a.threadCompose.Uploading() {
		return toastWithClear(a, "Upload in progress", 2*time.Second)
	}
	target := &a.compose
	if m.thread {
		target = &a.threadCompose
	}
	cmd, ok := a.attachPaths(target, m.paths)
	if !ok {
		return toastWithClear(a, "Could not read the chosen file", 3*time.Second)
	}
	return cmd
}
