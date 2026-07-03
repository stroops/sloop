package tmux

import "testing"

func TestStatusSideFormat(t *testing.T) {
	got := StatusSideFormat("/usr/bin/sloop", "right", "api__claude")
	want := "#(/usr/bin/sloop statusline right api__claude)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStatusLineEnabled(t *testing.T) {
	for _, off := range []string{"0", "off", "false", "no", "OFF"} {
		t.Setenv("SLOOP_STATUSLINE", off)
		if statusLineEnabled() {
			t.Errorf("SLOOP_STATUSLINE=%q should disable the status line", off)
		}
	}
	for _, on := range []string{"", "1", "on", "yes", "anything"} {
		t.Setenv("SLOOP_STATUSLINE", on)
		if !statusLineEnabled() {
			t.Errorf("SLOOP_STATUSLINE=%q should keep the status line on", on)
		}
	}
}
