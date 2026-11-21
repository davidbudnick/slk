package text

import "unicode"

// HasWordPrefix reports whether a word of s after its first starts
// with prefix, where a word starts after any character that is not a
// letter or digit: "pepe_ez" has the word prefix "ez", "geez" does
// not. Both are compared as given; fold them first to ignore case.
func HasWordPrefix(s, prefix string) bool {
	if prefix == "" {
		return false
	}
	boundary := false
	for i, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			boundary = true
			continue
		}
		if boundary && i > 0 && len(s)-i >= len(prefix) && s[i:i+len(prefix)] == prefix {
			return true
		}
		boundary = false
	}
	return false
}
