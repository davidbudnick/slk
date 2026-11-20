package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// chooserTitle is the prompt on the system's file chooser.
const chooserTitle = "Attach to a Slack message"

// macChooserScript is the AppleScript /attach runs on macOS: the
// standard "choose file" dialog, brought to the front, returning each
// picked file's path on a line of its own. Cancelling ends the script
// with error -128.
var macChooserScript = []string{
	"activate",
	`set picked to choose file with prompt "` + chooserTitle + `" with multiple selections allowed`,
	`set out to ""`,
	"repeat with f in picked",
	"set out to out & POSIX path of f & linefeed",
	"end repeat",
	"return out",
}

// fileChooser opens the system's file chooser for goos. lookPath and
// run are exec's, replaced in tests.
type fileChooser struct {
	goos     string
	lookPath func(name string) (string, error)
	run      func(name string, args ...string) (stdout, stderr string, err error)
}

// newFileChooser returns the file chooser for this system.
func newFileChooser() fileChooser {
	return fileChooser{goos: runtime.GOOS, lookPath: exec.LookPath, run: runCaptured}
}

// runCaptured runs a command to its end and returns what it wrote.
func runCaptured(name string, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// Choose opens the chooser and blocks until the user picks or cancels.
// It returns the picked paths, none for a cancel, and
// errors.ErrUnsupported when this system has no chooser slk can open.
func (c fileChooser) Choose() ([]string, error) {
	var name string
	var args []string
	switch c.goos {
	case "darwin":
		name = "osascript"
		for _, line := range macChooserScript {
			args = append(args, "-e", line)
		}
	case "linux", "freebsd", "openbsd", "netbsd":
		switch {
		case c.has("zenity"):
			name, args = "zenity", []string{"--file-selection", "--multiple", "--separator=\n", "--title=" + chooserTitle}
		case c.has("kdialog"):
			name, args = "kdialog", []string{"--getopenfilename", ".", "--multiple", "--separate-output", "--title", chooserTitle}
		}
	}
	if name == "" || !c.has(name) {
		return nil, errors.ErrUnsupported
	}
	stdout, stderr, err := c.run(name, args...)
	if err != nil {
		// Every chooser here exits non-zero on a cancel; osascript also
		// names it. Anything else on stderr is a real failure.
		var exit *exec.ExitError
		if errors.As(err, &exit) && (strings.Contains(stderr, "-128") || strings.TrimSpace(stderr) == "") {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr))
	}
	var paths []string
	for _, line := range strings.Split(stdout, "\n") {
		if line = strings.TrimRight(line, "\r"); line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

// has reports whether the named program is on PATH.
func (c fileChooser) has(name string) bool {
	_, err := c.lookPath(name)
	return err == nil
}
