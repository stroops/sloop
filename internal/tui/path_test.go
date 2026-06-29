package tui

import "testing"

func TestAbbreviateHome(t *testing.T) {
	t.Setenv("HOME", "/Users/me")
	cases := []struct{ in, want string }{
		{"/Users/me", "~"},
		{"/Users/me/code/api", "~/code/api"},
		{"/opt/other", "/opt/other"},
		{"/Users/melon/x", "/Users/melon/x"}, // prefix but not a path boundary
	}
	for _, c := range cases {
		if got := AbbreviateHome(c.in); got != c.want {
			t.Errorf("AbbreviateHome(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
