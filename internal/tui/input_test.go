package tui

import "testing"

func TestTrimLastRune(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"a", ""},
		{"abc", "ab"},
		{"gõ", "g"},     // multibyte tail (õ is 2 bytes) → drop the whole rune
		{"việt", "việ"}, // ệ is 3 bytes → drop the whole rune, keep the rest intact
		{"⏎", ""},       // 3-byte rune → empty
	}
	for _, c := range cases {
		if got := string(trimLastRune([]byte(c.in))); got != c.want {
			t.Errorf("trimLastRune(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
