package image

import (
	"bytes"
	"strings"
	"testing"
)

// tmux forwards a graphics upload only from a visible pane, so an image
// uploaded while another tmux window is up is lost. It is sent again
// when the pane is back, once.
func TestSetTerminalHidden_SendsAgainWhatWasUploadedMeanwhile(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1/default,1,0")
	saved := KittyOutput
	defer func() { KittyOutput = saved; SetTerminalHidden(false) }()
	var side bytes.Buffer
	KittyOutput = &side
	uploads := func(id string) int { return strings.Count(side.String(), "a=T,f=100,t=d,i="+id+",") }

	var before bytes.Buffer
	if err := emitKittyUpload(&before, 7, "AAAA", 2, 1); err != nil {
		t.Fatal(err)
	}
	SetTerminalHidden(true)
	if side.Len() != 0 {
		t.Fatalf("hiding wrote %d bytes, want none", side.Len())
	}
	var during bytes.Buffer
	if err := emitKittyUpload(&during, 8, "BBBB", 2, 1); err != nil {
		t.Fatal(err)
	}
	if err := emitKittyUpload(&during, 9, "CCCC", 4, 2); err != nil {
		t.Fatal(err)
	}
	if err := emitKittyUpload(&during, 8, "DDDD", 2, 1); err != nil {
		t.Fatal(err)
	}
	if side.Len() != 0 {
		t.Fatalf("an upload while hidden wrote %d bytes to the side channel, want none yet", side.Len())
	}

	SetTerminalHidden(false)
	if uploads("7") != 0 {
		t.Error("an image uploaded in view was sent again")
	}
	if uploads("8") != 1 || uploads("9") != 1 {
		t.Errorf("images uploaded while hidden were sent again %d and %d times, want once each", uploads("8"), uploads("9"))
	}
	if !strings.Contains(side.String(), "DDDD") || strings.Contains(side.String(), "BBBB") {
		t.Error("an id uploaded twice while hidden must be sent again with its newest image")
	}
	if !strings.Contains(side.String(), "\x1bPtmux;") {
		t.Error("the upload sent again is not wrapped for tmux")
	}

	side.Reset()
	SetTerminalHidden(false)
	var after bytes.Buffer
	if err := emitKittyUpload(&after, 10, "EEEE", 2, 1); err != nil {
		t.Fatal(err)
	}
	SetTerminalHidden(false)
	if side.Len() != 0 {
		t.Errorf("coming back twice, or an upload in view, wrote %d more bytes", side.Len())
	}
}

// Outside tmux a hidden terminal still receives its uploads.
func TestSetTerminalHidden_NothingSentAgainOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	saved := KittyOutput
	defer func() { KittyOutput = saved; SetTerminalHidden(false) }()
	var side bytes.Buffer
	KittyOutput = &side

	SetTerminalHidden(true)
	var during bytes.Buffer
	if err := emitKittyUpload(&during, 8, "BBBB", 2, 1); err != nil {
		t.Fatal(err)
	}
	SetTerminalHidden(false)
	if side.Len() != 0 {
		t.Errorf("outside tmux %d bytes were sent again, want none", side.Len())
	}
}
