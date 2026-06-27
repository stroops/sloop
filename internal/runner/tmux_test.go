package runner

import (
	"reflect"
	"strings"
	"testing"
)

func TestTmuxSessionNameSanitizes(t *testing.T) {
	got := TmuxSessionName("my-app", "claude")
	if got != "my_app__claude" {
		t.Fatalf("want my_app__claude, got %s", got)
	}
}

func TestBuildTmuxNewArgs(t *testing.T) {
	args := BuildTmuxNewArgs("backend__claude", Spec{Dir: "/tmp/backend", Command: "claude", Args: []string{"--resume"}})
	want := []string{"new-session", "-A", "-s", "backend__claude", "-c", "/tmp/backend", "claude", "--resume"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("want %v, got %v", want, args)
	}
}

func TestBuildTmuxAttachArgs(t *testing.T) {
	args := BuildTmuxAttachArgs("backend__claude")
	want := []string{"attach", "-t", "backend__claude"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("want %v, got %v", want, args)
	}
}

func TestDetachHintAndLine(t *testing.T) {
	if DetachHint() == "" {
		t.Fatal("DetachHint should not be empty")
	}
	if DetachLine() == "" || !strings.Contains(DetachLine(), "then d") {
		t.Fatalf("DetachLine = %q", DetachLine())
	}
}
