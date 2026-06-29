package tmux

import "testing"

func TestKeysEnabled(t *testing.T) {
	for _, off := range []string{"0", "off", "false", "no", "OFF"} {
		t.Setenv("SLOOP_KEYS", off)
		if keysEnabled() {
			t.Errorf("SLOOP_KEYS=%q should disable auto-bind", off)
		}
	}
	for _, on := range []string{"", "1", "on", "yes", "anything"} {
		t.Setenv("SLOOP_KEYS", on)
		if !keysEnabled() {
			t.Errorf("SLOOP_KEYS=%q should keep auto-bind on", on)
		}
	}
}

func TestParsePrefixKeys(t *testing.T) {
	raw := "bind-key    -T prefix p          previous-window\n" +
		"bind-key    -T prefix g          display-popup -E 'sloop ps'\n" +
		"bind-key    -T root   MouseDown1Pane select-pane\n"
	got := parsePrefixKeys(raw)
	if !got["p"] || !got["g"] {
		t.Fatalf("expected p and g bound, got %v", got)
	}
	if got["j"] {
		t.Fatalf("j should not be reported bound: %v", got)
	}
}

func TestPickFreeKey(t *testing.T) {
	if got := pickFreeKey([]string{"j", "a", "f", "p"}, map[string]bool{"j": true, "a": true}); got != "f" {
		t.Fatalf("got %q want f", got)
	}
	if got := pickFreeKey([]string{"h", "g"}, map[string]bool{}); got != "h" {
		t.Fatalf("got %q want h", got)
	}
	if got := pickFreeKey([]string{"j", "a"}, map[string]bool{"j": true, "a": true}); got != "" {
		t.Fatalf("all taken should give empty, got %q", got)
	}
}
