package commands

import (
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/tmux"
)

func TestParseEnvFlags(t *testing.T) {
	got, err := parseEnvFlags([]string{"K=V", "X=a=b"})
	if err != nil {
		t.Fatal(err)
	}
	if got["K"] != "V" || got["X"] != "a=b" {
		t.Fatalf("got %v", got)
	}
	if _, err := parseEnvFlags([]string{"noeq"}); err == nil {
		t.Fatal("want error for missing =")
	}
}

func TestExpandEnvValue(t *testing.T) {
	t.Setenv("HOME", "/home/me")
	cases := map[string]string{
		"~/.claude-sec": "/home/me/.claude-sec",
		"$HOME/x":       "/home/me/x",
		"${HOME}/y":     "/home/me/y",
		"/abs/path":     "/abs/path",
		"~notme/x":      "~notme/x", // only a leading ~/ expands
	}
	for in, want := range cases {
		if got := expandEnvValue(in); got != want {
			t.Fatalf("expandEnvValue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveInstance(t *testing.T) {
	t.Setenv("HOME", "/home/me")
	m := loadManifestsForTest(t) // resets HOME to a temp dir
	t.Setenv("HOME", "/home/me") // restore for expansion assertions
	profiles := map[string]config.Profile{
		"sec":   {Tool: "claude", Env: map[string]string{"CLAUDE_CONFIG_DIR": "~/.claude-sec"}},
		"bogus": {Tool: "nope", Env: nil},
	}

	t.Run("profile", func(t *testing.T) {
		r, err := resolveInstance("@sec", "", nil, profiles, m)
		if err != nil {
			t.Fatal(err)
		}
		if r.target != "claude" || r.instance != "sec" {
			t.Fatalf("got target=%q instance=%q", r.target, r.instance)
		}
		if r.env["CLAUDE_CONFIG_DIR"] != "/home/me/.claude-sec" {
			t.Fatalf("env not expanded: %v", r.env)
		}
	})
	t.Run("tool@instance", func(t *testing.T) {
		r, err := resolveInstance("claude@b", "", nil, profiles, m)
		if err != nil || r.target != "claude" || r.instance != "b" || r.env != nil {
			t.Fatalf("got %+v err %v", r, err)
		}
	})
	t.Run("plain tool", func(t *testing.T) {
		r, err := resolveInstance("claude", "", nil, profiles, m)
		if err != nil || r.target != "claude" || r.instance != "" {
			t.Fatalf("got %+v err %v", r, err)
		}
	})
	t.Run("unknown profile", func(t *testing.T) {
		if _, err := resolveInstance("@nope", "", nil, profiles, m); err == nil || !strings.Contains(err.Error(), "unknown profile") {
			t.Fatalf("want unknown profile, got %v", err)
		}
	})
	t.Run("profile bad tool", func(t *testing.T) {
		if _, err := resolveInstance("@bogus", "", nil, profiles, m); err == nil || !strings.Contains(err.Error(), "unknown tool") {
			t.Fatalf("want unknown tool, got %v", err)
		}
	})
	t.Run("bad left of @", func(t *testing.T) {
		if _, err := resolveInstance("zzz@x", "", nil, profiles, m); err == nil || !strings.Contains(err.Error(), "unknown tool") {
			t.Fatalf("want unknown tool, got %v", err)
		}
	})
	t.Run("name flag overrides instance", func(t *testing.T) {
		r, err := resolveInstance("@sec", "override", nil, profiles, m)
		if err != nil || r.instance != "override" {
			t.Fatalf("got %+v err %v", r, err)
		}
	})
	t.Run("name flag on plain tool", func(t *testing.T) {
		r, err := resolveInstance("claude", "x", nil, profiles, m)
		if err != nil || r.instance != "x" {
			t.Fatalf("got %+v err %v", r, err)
		}
	})
	t.Run("env flag merges and overrides", func(t *testing.T) {
		r, err := resolveInstance("@sec", "", []string{"CLAUDE_CONFIG_DIR=/flag", "EXTRA=1"}, profiles, m)
		if err != nil {
			t.Fatal(err)
		}
		if r.env["CLAUDE_CONFIG_DIR"] != "/flag" || r.env["EXTRA"] != "1" {
			t.Fatalf("env flag did not override/merge: %v", r.env)
		}
	})
	t.Run("instance cannot contain __", func(t *testing.T) {
		if _, err := resolveInstance("claude", "a__b", nil, profiles, m); err == nil || !strings.Contains(err.Error(), "__") {
			t.Fatalf("want __ error, got %v", err)
		}
	})
}

func TestNextFreeInstance(t *testing.T) {
	sess := func(names ...string) []tmux.Session {
		out := make([]tmux.Session, len(names))
		for i, n := range names {
			out[i] = tmux.Session{Name: n}
		}
		return out
	}
	if got := nextFreeInstance("repo", "claude", nil); got != "" {
		t.Fatalf("no sessions → empty, got %q", got)
	}
	if got := nextFreeInstance("repo", "claude", sess("repo__claude")); got != "2" {
		t.Fatalf("base taken → 2, got %q", got)
	}
	if got := nextFreeInstance("repo", "claude", sess("repo__claude", "repo__claude__2")); got != "3" {
		t.Fatalf("base+2 taken → 3, got %q", got)
	}
}
