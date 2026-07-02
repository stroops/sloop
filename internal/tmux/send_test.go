package tmux

import (
	"reflect"
	"testing"
)

func TestBuildTmuxSendTextArgs(t *testing.T) {
	got := BuildSendTextArgs("web__claude", "run the tests")
	want := []string{"send-keys", "-t", "=web__claude", "-l", "--", "run the tests"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// A message starting with "-" must still pass as text, not a flag.
	got = BuildSendTextArgs("s", "--help me")
	if got[len(got)-1] != "--help me" {
		t.Fatalf("message mangled: %v", got)
	}
}

func TestBuildTmuxSendEnterArgs(t *testing.T) {
	got := BuildSendEnterArgs("web__claude")
	want := []string{"send-keys", "-t", "=web__claude", "Enter"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
