package commands

import (
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
)

func TestProfileAdd(t *testing.T) {
	m := map[string]adapter.Manifest{"claude": {Name: "Claude"}}
	g := &config.Global{}

	if err := profileAdd(g, "sec", "claude", []string{"CLAUDE_CONFIG_DIR=/x"}, m); err != nil {
		t.Fatal(err)
	}
	p, ok := g.Profiles["sec"]
	if !ok || p.Tool != "claude" || p.Env["CLAUDE_CONFIG_DIR"] != "/x" {
		t.Fatalf("profile not stored: %+v", g.Profiles)
	}

	if err := profileAdd(g, "bad", "nope", nil, m); err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("want unknown tool error, got %v", err)
	}
	if err := profileAdd(g, "", "claude", nil, m); err == nil {
		t.Fatal("want error for empty name")
	}
}

func TestProfileRemove(t *testing.T) {
	g := &config.Global{Profiles: map[string]config.Profile{"sec": {Tool: "claude"}}}
	if err := profileRemove(g, "nope"); err == nil || !strings.Contains(err.Error(), "no profile") {
		t.Fatalf("want no-profile error, got %v", err)
	}
	if err := profileRemove(g, "sec"); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.Profiles["sec"]; ok {
		t.Fatal("profile not removed")
	}
}

func TestRenderProfileList(t *testing.T) {
	if got := renderProfileList(nil); !strings.Contains(got, "profile add") {
		t.Fatalf("empty list should hint, got %q", got)
	}
	got := renderProfileList(map[string]config.Profile{
		"sec": {Tool: "claude", Env: map[string]string{"CLAUDE_CONFIG_DIR": "/x"}},
	})
	if !strings.Contains(got, "sec") || !strings.Contains(got, "claude") || !strings.Contains(got, "CLAUDE_CONFIG_DIR") {
		t.Fatalf("list missing fields: %q", got)
	}
}
