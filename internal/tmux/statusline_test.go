package tmux

import "testing"

func TestStatusRightFormat(t *testing.T) {
	got := StatusRightFormat("/usr/bin/sloop", "api__claude")
	want := "#(/usr/bin/sloop statusline api__claude)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
