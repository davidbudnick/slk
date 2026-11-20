// Package slackfmt formats Slack-internal identifiers (channel names,
// etc.) for human display.
package slackfmt

import "strings"

// FormatMPDMName converts Slack's raw multi-party DM channel name
// (e.g. "mpdm-grant--myles--ray-1") into a human-readable participant
// list ("Myles Williamson, Ray Bradbury" when the user is grant).
//
// Slack's mpdm naming convention is:
//
//	mpdm-<handle1>--<handle2>--...--<handleN>-<index>
//
// where each handle is a user's `name` field (the @-handle, NOT the
// display name) and `<index>` is a small integer disambiguator. The
// double-dash (`--`) is the delimiter; a single dash is a legitimate
// character within a handle.
//
// self is the current user's handle. It is left out of the list,
// since the user is in every group DM they can see, unless no other
// participant is left. Pass "" to keep every participant.
//
// lookup is called for each parsed handle and should return the
// corresponding display name. If lookup is nil or returns an empty
// string for a handle, the handle itself is used as a fallback.
//
// If name does not match the mpdm format (no "mpdm-" prefix, empty
// body, etc.), the original string is returned unchanged so callers
// can use this unconditionally.
func FormatMPDMName(name, self string, lookup func(handle string) string) string {
	handles := mpdmHandles(name, self)
	if len(handles) == 0 {
		return name
	}
	displays := make([]string, 0, len(handles))
	for _, h := range handles {
		var d string
		if lookup != nil {
			d = lookup(h)
		}
		if d == "" {
			d = h
		}
		displays = append(displays, d)
	}
	return strings.Join(displays, ", ")
}

// MPDMOthers is the number of people FormatMPDMName lists for name: the
// figure Slack draws on a group DM's icon. 0 if name is not an mpdm
// name.
func MPDMOthers(name, self string) int {
	return len(mpdmHandles(name, self))
}

// mpdmHandles is the handles in a raw mpdm name, without self unless
// nobody else is left. Nil if name is not an mpdm name.
func mpdmHandles(name, self string) []string {
	const prefix = "mpdm-"
	if !strings.HasPrefix(name, prefix) {
		return nil
	}
	body := name[len(prefix):]

	// Strip the trailing "-<digits>" index, if present.
	if i := strings.LastIndexByte(body, '-'); i >= 0 && i < len(body)-1 {
		if isAllDigits(body[i+1:]) {
			body = body[:i]
		}
	}

	var all, others []string
	for _, h := range strings.Split(body, "--") {
		if h == "" {
			continue
		}
		all = append(all, h)
		if h != self {
			others = append(others, h)
		}
	}
	if self != "" && len(others) > 0 {
		return others
	}
	return all
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
