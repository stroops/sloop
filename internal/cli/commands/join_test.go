package commands

import "testing"

func TestJoinWith(t *testing.T) {
	if got := joinWith(" | ", "a", "b", "c"); got != "a | b | c" {
		t.Fatalf("got %q", got)
	}
	// Empty items are dropped, not left as a stray/doubled separator.
	if got := joinWith(" | ", "a", "", "c"); got != "a | c" {
		t.Fatalf("skip middle empty: got %q", got)
	}
	if got := joinWith(" | ", "", "b", ""); got != "b" {
		t.Fatalf("skip leading/trailing empty: got %q", got)
	}
	if got := joinWith(" | "); got != "" {
		t.Fatalf("no items → empty, got %q", got)
	}
	if got := joinWith(" | ", "", ""); got != "" {
		t.Fatalf("all empty → empty, got %q", got)
	}
}

func TestJoinWithDot(t *testing.T) {
	if got := joinWithDot("Opus", "ctx 45%"); got != "Opus · ctx 45%" {
		t.Fatalf("got %q", got)
	}
	if got := joinWithDot("Opus", ""); got != "Opus" {
		t.Fatalf("one empty → the other alone, got %q", got)
	}
	if got := joinWithDot("", ""); got != "" {
		t.Fatalf("both empty → empty, got %q", got)
	}
}

func TestJoinWithSpace(t *testing.T) {
	if got := joinWithSpace("[y]es", "[n]o"); got != "[y]es [n]o" {
		t.Fatalf("got %q", got)
	}
	if got := joinWithSpace("solo", ""); got != "solo" {
		t.Fatalf("got %q", got)
	}
}
