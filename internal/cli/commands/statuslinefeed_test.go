package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/fleetstate"
)

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// Claude-shaped payload: context comes as token counts to sum against the
// window size (the mapping claude.yaml declares).
func TestExtractStatusInfoClaudeShape(t *testing.T) {
	doc := decode(t, `{
		"model": {"display_name": "Opus"},
		"workspace": {"current_dir": "/tmp/repo"},
		"context_window": {
			"context_window_size": 200000,
			"current_usage": {
				"input_tokens": 50000,
				"cache_creation_input_tokens": 20000,
				"cache_read_input_tokens": 30000
			}
		}
	}`)
	p := adapter.StatusLinePayload{
		Model: "model.display_name",
		Cwd:   "workspace.current_dir",
		ContextUsed: []string{
			"context_window.current_usage.input_tokens",
			"context_window.current_usage.cache_creation_input_tokens",
			"context_window.current_usage.cache_read_input_tokens",
		},
		ContextSize: "context_window.context_window_size",
	}
	model, pct := extractStatusInfo(doc, p)
	if model != "Opus" || pct != 50 {
		t.Fatalf("got %q %d, want Opus 50", model, pct)
	}
}

// agy-shaped payload: a ready-made percentage plus an agent state that maps
// to a sloop status.
func TestExtractStatusInfoAgyShape(t *testing.T) {
	doc := decode(t, `{
		"model": {"display_name": "Gemini 3 Pro"},
		"agent_state": "tool_use",
		"context_window": {"used_percentage": 37.6}
	}`)
	spec := adapter.StatusLineSpec{
		Payload: adapter.StatusLinePayload{
			Model:      "model.display_name",
			ContextPct: "context_window.used_percentage",
			State:      "agent_state",
		},
		States: map[string]string{"tool_use": "working", "idle": "idle"},
	}
	model, pct := extractStatusInfo(doc, spec.Payload)
	if model != "Gemini 3 Pro" || pct != 38 {
		t.Fatalf("got %q %d, want Gemini 3 Pro 38", model, pct)
	}
	if st := extractStatusState(doc, spec); st != "working" {
		t.Fatalf("state = %q, want working", st)
	}
}

// Claude-shaped rate limit: a ready used% plus an absolute reset timestamp.
func TestExtractRateLimitClaudeShape(t *testing.T) {
	resetAt := time.Now().Add(45*time.Minute + 30*time.Second).Unix() // clear of the 45m floor boundary
	doc := decode(t, fmt.Sprintf(`{"rate_limits":{"five_hour":{"used_percentage":23.5,"resets_at":%d}}}`, resetAt))
	p := adapter.StatusLinePayload{
		RateLimitUsedPct:  "rate_limits.five_hour.used_percentage",
		RateLimitResetsAt: "rate_limits.five_hour.resets_at",
	}
	pct, reset := extractRateLimit(doc, p)
	if pct != 24 { // rounds 23.5 → 24
		t.Fatalf("pct = %d, want 24", pct)
	}
	if reset != "45m" {
		t.Fatalf("reset = %q, want 45m", reset)
	}
}

// agy-shaped rate limit: a remaining fraction to invert plus a relative
// seconds-until-reset.
func TestExtractRateLimitAgyShape(t *testing.T) {
	doc := decode(t, `{"quota":{"gemini-5h":{"remaining_fraction":0.7,"reset_in_seconds":1800}}}`)
	p := adapter.StatusLinePayload{
		RateLimitRemainingFrac: "quota.gemini-5h.remaining_fraction",
		RateLimitResetsIn:      "quota.gemini-5h.reset_in_seconds",
	}
	pct, reset := extractRateLimit(doc, p)
	if pct != 30 { // 1 - 0.7 = 0.3 remaining used → 30% used
		t.Fatalf("pct = %d, want 30", pct)
	}
	if reset != "30m" {
		t.Fatalf("reset = %q, want 30m", reset)
	}
}

// No mapping declared, or the mapped fields are absent from the payload —
// both must degrade to (0, ""), never an error.
func TestExtractRateLimitMissing(t *testing.T) {
	if pct, reset := extractRateLimit(decode(t, `{}`), adapter.StatusLinePayload{}); pct != 0 || reset != "" {
		t.Fatalf("no mapping = (%d, %q)", pct, reset)
	}
	p := adapter.StatusLinePayload{RateLimitUsedPct: "rate_limits.five_hour.used_percentage"}
	if pct, reset := extractRateLimit(decode(t, `{"something":"else"}`), p); pct != 0 || reset != "" {
		t.Fatalf("field absent from payload = (%d, %q)", pct, reset)
	}
}

// A payload with none of the mapped fields must degrade to zero values —
// empty segments, never placeholders or errors.
func TestExtractStatusInfoMissingFields(t *testing.T) {
	doc := decode(t, `{"something": "else"}`)
	p := adapter.StatusLinePayload{Model: "model.display_name", ContextPct: "context_window.used_percentage"}
	model, pct := extractStatusInfo(doc, p)
	if model != "" || pct != 0 {
		t.Fatalf("want empty on missing fields, got %q %d", model, pct)
	}
	if st := extractStatusState(doc, adapter.StatusLineSpec{}); st != "" {
		t.Fatalf("state on empty spec = %q", st)
	}
}

func TestDefaultInlineStatus(t *testing.T) {
	doc := decode(t, `{"model":{"display_name":"Opus"},"workspace":{"current_dir":"/tmp/myrepo"}}`)
	p := adapter.StatusLinePayload{Model: "model.display_name", Cwd: "workspace.current_dir"}
	out := defaultInlineStatus(doc, p)
	if !strings.Contains(out, "myrepo") || !strings.Contains(out, "Opus") {
		t.Fatalf("default line = %q", out)
	}
}

func TestInstallStatuslineFeedFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	changed, installed, err := installStatuslineFeed(path, "sloop statusline feed claude")
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if strings.Contains(installed, "--chain") {
		t.Fatalf("no chain expected on a fresh install, got %q", installed)
	}
	b, _ := os.ReadFile(path)
	doc := decode(t, string(b))
	sl, _ := doc["statusLine"].(map[string]any)
	if sl["type"] != "command" || sl["command"] != "sloop statusline feed claude" {
		t.Fatalf("statusLine = %#v", sl)
	}
}

// A user's existing statusline command must be preserved via --chain, other
// settings keys untouched, and a re-install must be a no-op.
func TestInstallStatuslineFeedChainsAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	orig := `{"statusLine":{"type":"command","command":"~/.claude/statusline.sh","padding":0},"model":"opus"}`
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, installed, err := installStatuslineFeed(path, "sloop statusline feed claude")
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	want := "sloop statusline feed claude --chain '~/.claude/statusline.sh'"
	if installed != want {
		t.Fatalf("installed = %q, want %q", installed, want)
	}
	b, _ := os.ReadFile(path)
	doc := decode(t, string(b))
	if doc["model"] != "opus" {
		t.Fatalf("unrelated keys must survive: %v", doc)
	}
	if sl, _ := doc["statusLine"].(map[string]any); sl["padding"] != float64(0) {
		t.Fatalf("statusLine extras must survive: %#v", sl)
	}
	changed, _, err = installStatuslineFeed(path, "sloop statusline feed claude")
	if err != nil || changed {
		t.Fatalf("second install must be a no-op, changed=%v err=%v", changed, err)
	}
}

// The feed fires on every render of the tool's own statusline — often many
// times a minute with an unchanged payload — so an identical payload must not
// rewrite the marker file. A payload with new info must still get through.
func TestFeedSkipsRedundantWrites(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SLOOP_SESSION", "ws__claude")

	payload := []byte(`{
		"model": {"display_name": "Opus"},
		"workspace": {"current_dir": "/tmp"},
		"context_window": {
			"context_window_size": 100,
			"current_usage": {"input_tokens": 30, "cache_creation_input_tokens": 0, "cache_read_input_tokens": 0}
		}
	}`)
	feed := func(body []byte) {
		cmd := &cobra.Command{}
		cmd.SetIn(bytes.NewReader(body))
		cmd.SetOut(io.Discard)
		if err := statuslineFeedCmd.RunE(cmd, []string{"claude"}); err != nil {
			t.Fatal(err)
		}
	}

	feed(payload)
	stateDir, err := fleetstate.Dir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(stateDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected exactly one marker, got %v (err=%v)", entries, err)
	}
	markerPath := filepath.Join(stateDir, entries[0].Name())
	before, err := os.Stat(markerPath)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Millisecond) // give a rewrite a distinguishable mtime
	feed(payload)
	afterSame, err := os.Stat(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(afterSame.ModTime()) {
		t.Fatalf("identical payload rewrote the marker: %v -> %v", before.ModTime(), afterSame.ModTime())
	}

	changed := []byte(`{
		"model": {"display_name": "Opus"},
		"workspace": {"current_dir": "/tmp"},
		"context_window": {
			"context_window_size": 100,
			"current_usage": {"input_tokens": 60, "cache_creation_input_tokens": 0, "cache_read_input_tokens": 0}
		}
	}`)
	time.Sleep(5 * time.Millisecond)
	feed(changed)
	afterChanged, err := os.Stat(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !afterChanged.ModTime().After(afterSame.ModTime()) {
		t.Fatalf("changed payload (30%%->60%%) must rewrite the marker")
	}
	_, pct := fleetstate.Info("ws__claude")
	if pct != 60 {
		t.Fatalf("ctxPct = %d, want 60", pct)
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote(`echo 'hi'`); got != `'echo '\''hi'\'''` {
		t.Fatalf("got %s", got)
	}
}

// The built-in claude/agy manifests must declare a working statusline spec —
// this is what `sloop statusline install` relies on.
func TestBuiltinStatuslineSpecs(t *testing.T) {
	ms, err := adapter.LoadBuiltin()
	if err != nil {
		t.Fatal(err)
	}
	c := ms["claude"].StatusLine
	if c.Install != "settings-json" || c.Payload.Model == "" || len(c.Payload.ContextUsed) == 0 || c.Payload.ContextSize == "" {
		t.Fatalf("claude statusline spec incomplete: %+v", c)
	}
	if c.Payload.RateLimitUsedPct == "" || c.Payload.RateLimitResetsAt == "" {
		t.Fatalf("claude rate-limit mapping incomplete: %+v", c.Payload)
	}
	a := ms["agy"].StatusLine
	if a.Install != "settings-json" || a.Payload.ContextPct == "" || a.Payload.State == "" || len(a.States) == 0 {
		t.Fatalf("agy statusline spec incomplete: %+v", a)
	}
	if a.Payload.RateLimitRemainingFrac == "" || a.Payload.RateLimitResetsIn == "" {
		t.Fatalf("agy rate-limit mapping incomplete: %+v", a.Payload)
	}
}
