package main

import (
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// chooserWith is a fileChooser for goos whose PATH holds have and whose
// program writes stdout and stderr and ends with err. ran collects the
// program's name and arguments.
func chooserWith(goos string, have []string, stdout, stderr string, err error, ran *[]string) fileChooser {
	return fileChooser{
		goos: goos,
		lookPath: func(name string) (string, error) {
			for _, h := range have {
				if h == name {
					return "/usr/bin/" + name, nil
				}
			}
			return "", exec.ErrNotFound
		},
		run: func(name string, args ...string) (string, string, error) {
			*ran = append([]string{name}, args...)
			return stdout, stderr, err
		},
	}
}

func TestFileChooser_MacOS(t *testing.T) {
	var ran []string
	paths, err := chooserWith("darwin", []string{"osascript"}, "/Users/ann/Screen Shot 1.png\n/Users/ann/b.txt\n\n", "", nil, &ran).Choose()
	if err != nil || !reflect.DeepEqual(paths, []string{"/Users/ann/Screen Shot 1.png", "/Users/ann/b.txt"}) {
		t.Fatalf("Choose = %q, %v; want the two picked paths", paths, err)
	}
	script := strings.Join(ran, " ")
	if ran[0] != "osascript" || !strings.Contains(script, "choose file") || !strings.Contains(script, "multiple selections allowed") || !strings.Contains(script, "POSIX path") {
		t.Errorf("ran %q; want osascript's choose file, several files allowed, returning POSIX paths", ran)
	}

	paths, err = chooserWith("darwin", []string{"osascript"}, "", "execution error: User canceled. (-128)\n", &exec.ExitError{}, &ran).Choose()
	if err != nil || paths != nil {
		t.Errorf("a cancel = %q, %v; want no paths and no error", paths, err)
	}

	_, err = chooserWith("darwin", []string{"osascript"}, "", "execution error: Not authorized. (-1743)\n", &exec.ExitError{}, &ran).Choose()
	if err == nil || !strings.Contains(err.Error(), "-1743") {
		t.Errorf("a failure = %v; want an error carrying osascript's message", err)
	}

	if _, err = chooserWith("darwin", nil, "", "", nil, &ran).Choose(); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("without osascript: err = %v; want ErrUnsupported", err)
	}
}

func TestFileChooser_Linux(t *testing.T) {
	var ran []string
	paths, err := chooserWith("linux", []string{"kdialog", "zenity"}, "/home/ann/a.png\n", "", nil, &ran).Choose()
	if err != nil || !reflect.DeepEqual(paths, []string{"/home/ann/a.png"}) || ran[0] != "zenity" {
		t.Errorf("Choose = %q, %v via %q; want the path from zenity, tried first", paths, err, ran)
	}

	paths, err = chooserWith("linux", []string{"kdialog"}, "/home/ann/a.png\n/home/ann/b.png\n", "", nil, &ran).Choose()
	if err != nil || len(paths) != 2 || ran[0] != "kdialog" {
		t.Errorf("Choose = %q, %v via %q; want two paths from kdialog", paths, err, ran)
	}

	paths, err = chooserWith("linux", []string{"zenity"}, "", "", &exec.ExitError{}, &ran).Choose()
	if err != nil || paths != nil {
		t.Errorf("a cancel = %q, %v; want no paths and no error", paths, err)
	}

	for _, goos := range []string{"linux", "windows", "plan9"} {
		if _, err := chooserWith(goos, nil, "", "", nil, &ran).Choose(); !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("%s with no chooser: err = %v; want ErrUnsupported", goos, err)
		}
	}
}
