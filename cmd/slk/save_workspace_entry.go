package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// saveWorkspaceEntry rewrites or appends `key = value` in
// [workspaces.<tomlKey>]; value is already TOML-encoded. It is a textual
// line rewrite rather than a TOML re-marshal, so user comments and field
// ordering survive the write. With no such block, it appends one keyed
// by the team ID (the legacy form), labelled with the team's name.
func saveWorkspaceEntry(configPath, tomlKey, teamID, teamName, key, value string) error {
	configWriteMu.Lock()
	defer configWriteMu.Unlock()

	data, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			return err
		}
		data = nil
	} else if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	entry := key + " = " + value

	header := fmt.Sprintf("[workspaces.%s]", tomlKey)
	sectionStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			sectionStart = i
			break
		}
	}

	if sectionStart < 0 {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		safeName := sanitizeComment(teamName)
		if safeName == "" {
			safeName = teamID
		}
		lines = append(lines, "# "+safeName, fmt.Sprintf("[workspaces.%s]", teamID), entry)
		return writeConfigAtomic(configPath, []byte(strings.Join(lines, "\n")))
	}

	first, last := workspaceEntrySpan(lines, sectionStart, key)
	if first < 0 {
		first, last = sectionStart+1, sectionStart
	}
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:first]...)
	newLines = append(newLines, entry)
	newLines = append(newLines, lines[last+1:]...)
	return writeConfigAtomic(configPath, []byte(strings.Join(newLines, "\n")))
}

// workspaceEntrySpan finds key among the entries directly under the
// table header at lines[header]: its first and last line (they differ
// for an array written over several lines), or -1 when the table has no
// such entry. The table runs to the next header, blank lines included.
func workspaceEntrySpan(lines []string, header int, key string) (first, last int) {
	first = -1
	depth := 0
	for i := header + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if depth == 0 {
			if strings.HasPrefix(t, "[") {
				break
			}
			if name, _, ok := strings.Cut(t, "="); ok && strings.TrimSpace(name) == key {
				first = i
			}
		}
		depth = max(depth+arrayDepthDelta(t), 0)
		if first >= 0 && depth == 0 {
			return first, i
		}
	}
	return first, first
}

// arrayDepthDelta is how many more arrays line opens than it closes,
// ignoring brackets inside strings and comments.
func arrayDepthDelta(line string) int {
	delta := 0
	var quote rune
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			escaped = false
		case quote == '"' && r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#':
			return delta
		case r == '[':
			delta++
		case r == ']':
			delta--
		}
	}
	return delta
}

// tomlStringArray returns items as a one-line TOML array of basic
// strings.
func tomlStringArray(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = tomlString(item)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// saveWorkspaceCollapsedSections rewrites or appends the
// collapsed_sections entry in [workspaces.<tomlKey>].
func saveWorkspaceCollapsedSections(configPath, tomlKey, teamID, teamName string, names []string) error {
	return saveWorkspaceEntry(configPath, tomlKey, teamID, teamName, "collapsed_sections", tomlStringArray(names))
}
