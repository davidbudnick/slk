package image

import "sync"

// hiddenUpload is what emitKittyUpload needs to send an image again.
type hiddenUpload struct {
	payload    string
	cols, rows int
}

// hiddenUploads holds the uploads written while the pane slk draws in
// was out of view, by image id.
var hiddenUploads struct {
	sync.Mutex
	hidden  bool
	uploads map[uint32]hiddenUpload
}

// SetTerminalHidden records whether the pane slk draws in may be out of
// view, and on its return sends again every image uploaded meanwhile.
// tmux forwards a passthrough sequence only from a visible pane
// (allow-passthrough on), so an image uploaded while another tmux window
// is up never reaches the terminal and its cells stay blank. Outside
// tmux nothing is dropped and nothing is sent again.
func SetTerminalHidden(hidden bool) {
	hiddenUploads.Lock()
	hiddenUploads.hidden = hidden
	var missed map[uint32]hiddenUpload
	if !hidden {
		missed, hiddenUploads.uploads = hiddenUploads.uploads, nil
	}
	hiddenUploads.Unlock()
	for id, up := range missed {
		_ = emitKittyUpload(KittyOutput, id, up.payload, up.cols, up.rows)
	}
}

// noteHiddenUpload remembers an upload written while the pane was out
// of view, for SetTerminalHidden to send again.
func noteHiddenUpload(id uint32, payload string, cols, rows int) {
	if !inTmux() {
		return
	}
	hiddenUploads.Lock()
	defer hiddenUploads.Unlock()
	if !hiddenUploads.hidden {
		return
	}
	if hiddenUploads.uploads == nil {
		hiddenUploads.uploads = map[uint32]hiddenUpload{}
	}
	hiddenUploads.uploads[id] = hiddenUpload{payload: payload, cols: cols, rows: rows}
}
