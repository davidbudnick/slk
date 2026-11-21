package messages

import (
	"sync"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// ShowMoreKey is the reaction hit key of an attachment's "Show more" row.
const ShowMoreKey = "\x00more"

// unfolded holds the ts of each message whose folded attachment text is shown in full.
var unfolded sync.Map

// Unfolded reports whether the message's folded attachment text is shown in full.
func Unfolded(ts string) bool {
	_, ok := unfolded.Load(ts)
	return ok
}

// ToggleFold shows the message's folded attachment text in full, or folds it again.
func ToggleFold(ts string) {
	if _, was := unfolded.LoadAndDelete(ts); !was {
		unfolded.Store(ts, struct{}{})
	}
}

// HasFold reports whether msg has attachment text folded behind "Show more".
func HasFold(msg MessageItem) bool {
	return CurrentLook().Slack && blockkit.HasFold(msg.LegacyAttachments)
}
