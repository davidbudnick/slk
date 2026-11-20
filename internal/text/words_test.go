package text

import "testing"

func TestHasWordPrefix(t *testing.T) {
	tests := []struct {
		s, prefix string
		want      bool
	}{
		{"pepe_ez", "ez", true},
		{"soc-handover", "ha", true},
		{"a.b c", "c", true},
		{"geez-omg", "ez", false},
		{"geez-omg", "omg", true},
		{"bezos2", "ez", false},
		{"ez_llama", "ez", false}, // the name's own start is not a later word
		{"ez_ez", "ez", true},
		{"pepe__ez", "ez", true},
		{"pepe_e", "ez", false},
		{"pepe_ez", "", false},
		{"", "ez", false},
		{"über_ärger", "är", true},
	}
	for _, tt := range tests {
		if got := HasWordPrefix(tt.s, tt.prefix); got != tt.want {
			t.Errorf("HasWordPrefix(%q, %q) = %v; want %v", tt.s, tt.prefix, got, tt.want)
		}
	}
}
