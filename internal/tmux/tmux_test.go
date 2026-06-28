package tmux

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/runner"
)

func TestTmuxSessionNameSanitizes(t *testing.T) {
	got := SessionName("my-app", "claude")
	if got != "my_app__claude" {
		t.Fatalf("want my_app__claude, got %s", got)
	}
}

func TestInstanceName(t *testing.T) {
	if got := InstanceName("repo", "claude", ""); got != "repo__claude" {
		t.Fatalf("empty instance: got %s", got)
	}
	if got := InstanceName("repo", "claude", "sec"); got != "repo__claude__sec" {
		t.Fatalf("named instance: got %s", got)
	}
	if got := InstanceName("repo", "claude", "a b"); got != "repo__claude__a_b" {
		t.Fatalf("sanitized instance: got %s", got)
	}
}

func TestEnvPrefix(t *testing.T) {
	if envPrefix(nil) != nil {
		t.Fatal("empty env should yield nil prefix")
	}
	got := envPrefix(map[string]string{"B": "2", "A": "1"})
	want := []string{"env", "A=1", "B=2"} // sorted by key
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestBuildNewDetachedArgs(t *testing.T) {
	plain := buildNewDetachedArgs("repo__claude", "/tmp/repo", nil, "claude", []string{"--resume"})
	wantPlain := []string{"new-session", "-d", "-s", "repo__claude", "-c", "/tmp/repo", "claude", "--resume"}
	if !reflect.DeepEqual(plain, wantPlain) {
		t.Fatalf("plain: got %v want %v", plain, wantPlain)
	}
	withEnv := buildNewDetachedArgs("repo__claude__sec", "/tmp/repo", map[string]string{"CLAUDE_CONFIG_DIR": "/x"}, "claude", nil)
	wantEnv := []string{"new-session", "-d", "-s", "repo__claude__sec", "-c", "/tmp/repo", "env", "CLAUDE_CONFIG_DIR=/x", "claude"}
	if !reflect.DeepEqual(withEnv, wantEnv) {
		t.Fatalf("withEnv: got %v want %v", withEnv, wantEnv)
	}
}

func TestBuildTmuxNewArgs(t *testing.T) {
	args := BuildNewArgs("backend__claude", runner.Spec{Dir: "/tmp/backend", Command: "claude", Args: []string{"--resume"}})
	want := []string{"new-session", "-A", "-s", "backend__claude", "-c", "/tmp/backend", "claude", "--resume"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("want %v, got %v", want, args)
	}
}

func TestBuildTmuxAttachArgs(t *testing.T) {
	args := BuildAttachArgs("backend__claude")
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

func TestResolveBin(t *testing.T) {
	// env override wins.
	if b := resolveBin("psmux", func(string) bool { return false }); b != "psmux" {
		t.Fatalf("env override: got %q", b)
	}
	// prefer tmux when both present.
	if b := resolveBin("", func(string) bool { return true }); b != "tmux" {
		t.Fatalf("prefer tmux: got %q", b)
	}
	// fall back to psmux when only it is present.
	if b := resolveBin("", func(c string) bool { return c == "psmux" }); b != "psmux" {
		t.Fatalf("psmux fallback: got %q", b)
	}
	// default when nothing found.
	if b := resolveBin("", func(string) bool { return false }); b != "tmux" {
		t.Fatalf("default: got %q", b)
	}
}

func TestBuildKillArgs(t *testing.T) {
	got := BuildKillArgs("web__claude")
	want := []string{"kill-session", "-t", "web__claude"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
