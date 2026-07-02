package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/fleetstate"
	"github.com/stroops/sloop/internal/tmux"
)

// The status bar is rendered by tmux's #(), which captures stdout only. cobra's
// cmd.Print writes to stderr, so the command must write to stdout explicitly;
// guard that, or the status bar silently shows nothing.
func TestStatuslineCommandWritesToStdout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out, errb bytes.Buffer
	statuslineCmd.SetOut(&out)
	statuslineCmd.SetErr(&errb)
	if err := statuslineCmd.RunE(statuslineCmd, []string{"myrepo__claude"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "⚓ myrepo claude") {
		t.Fatalf("stdout = %q, want the status line", out.String())
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", errb.String())
	}
}

func TestRenderStatusline(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no markers; no live session → unknown
	out := renderStatusline("myrepo__claude")
	if !strings.Contains(out, "⚓ myrepo claude") {
		t.Fatalf("statusline = %q", out)
	}
	if renderStatusline("") != "" {
		t.Fatal("empty session → empty")
	}
}

// Everything worth knowing at a glance — identity, status, model, context,
// branch — lives on the left, since tmux truncates status-right first on a
// narrow terminal.
func TestRenderStatuslineLeftCarriesIdentityAndAmbientInfo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	session := "myrepo__claude"

	if err := fleetstate.WriteInfo(session, "Opus", 45); err != nil {
		t.Fatal(err)
	}
	left := renderStatuslineLeft(session)
	for _, want := range []string{"⚓", "myrepo·claude", "Opus", "45%"} {
		if !strings.Contains(left, want) {
			t.Fatalf("left = %q, want it to contain %q", left, want)
		}
	}
	if renderStatuslineLeft("") != "" {
		t.Fatal("empty session → empty")
	}
}

// The right side is just the rotating hint now — model/context/branch moved
// to the left, so it must not repeat them.
func TestRenderStatuslineRightIsHintOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	session := "myrepo__claude"
	if err := fleetstate.WriteInfo(session, "Opus", 45); err != nil {
		t.Fatal(err)
	}
	right := renderStatuslineRight(session)
	if strings.Contains(right, "Opus") || strings.Contains(right, "45%") {
		t.Fatalf("right side must not repeat identity/ambient info, got %q", right)
	}
}

func TestWaitingBadge(t *testing.T) {
	if got := waitingBadge(0, ""); got != "" {
		t.Fatalf("zero should be empty, got %q", got)
	}
	if got := waitingBadge(-1, ""); got != "" {
		t.Fatalf("negative should be empty, got %q", got)
	}
	got := waitingBadge(2, " → Ctrl+b j")
	if !strings.Contains(got, "2 waiting") || !strings.Contains(got, "fg=yellow") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "→ Ctrl+b j") {
		t.Fatalf("badge should carry the hint, got %q", got)
	}
}

func TestPeekHint(t *testing.T) {
	if got := peekHint("Ctrl+b", ""); got != "" {
		t.Fatalf("no key should give no hint, got %q", got)
	}
	if got := peekHint("Ctrl+b", "j"); got != " → Ctrl+b j" {
		t.Fatalf("got %q", got)
	}
}

func TestRotatingHint(t *testing.T) {
	if rotatingHint(nil, time.Unix(0, 0)) != "" {
		t.Fatal("no hints → empty")
	}
	hints := []string{"a", "b", "c"}
	if got := rotatingHint(hints, time.Unix(0, 0)); got != "a" {
		t.Fatalf("slot 0 = %q", got)
	}
	if got := rotatingHint(hints, time.Unix(20, 0)); got != "b" {
		t.Fatalf("slot 1 = %q", got)
	}
	// Same 20s slot → same hint (no flicker between renders).
	if rotatingHint(hints, time.Unix(21, 0)) != rotatingHint(hints, time.Unix(39, 0)) {
		t.Fatal("hint must be stable within a slot")
	}
}

func TestContextSegment(t *testing.T) {
	if got := contextSegment(45); !strings.Contains(got, "45%") || !strings.Contains(got, "colour245") {
		t.Fatalf("comfortable usage should be dim: %q", got)
	}
	if !strings.Contains(contextSegment(75), "yellow") {
		t.Fatal("75% should warn")
	}
	if !strings.Contains(contextSegment(95), "red") {
		t.Fatal("95% should alarm")
	}
}

func TestContextBar(t *testing.T) {
	if got := contextBar(0); got != "░░░░░░░░" {
		t.Fatalf("0%% = %q", got)
	}
	if got := contextBar(100); got != "████████" {
		t.Fatalf("100%% = %q", got)
	}
	if got := contextBar(50); got != "████░░░░" {
		t.Fatalf("50%% = %q", got)
	}
	// Never overflow the bar width even if pct is somehow out of range.
	if got := contextBar(150); got != "████████" {
		t.Fatalf("150%% (clamped) = %q", got)
	}
}

// SLOOP_NERD_FONTS opts into the Nerd Font glyph set; anything else (unset
// included) keeps the universally-safe Unicode default.
func TestActiveIcons(t *testing.T) {
	t.Setenv("SLOOP_NERD_FONTS", "")
	if activeIcons() != iconsUnicode {
		t.Fatal("unset → unicode icons")
	}
	t.Setenv("SLOOP_NERD_FONTS", "1")
	if activeIcons() != iconsNerd {
		t.Fatal("SLOOP_NERD_FONTS=1 → nerd icons")
	}
	t.Setenv("SLOOP_NERD_FONTS", "true")
	if activeIcons() != iconsNerd {
		t.Fatal("SLOOP_NERD_FONTS=true → nerd icons")
	}
	t.Setenv("SLOOP_NERD_FONTS", "bogus")
	if activeIcons() != iconsUnicode {
		t.Fatal("unrecognized value → falls back to unicode icons")
	}
}

// gitBranch must resolve a normal checkout and a worktree-style `.git` file
// without spawning git, and stay silent on detached HEAD / non-repos.
func TestGitBranch(t *testing.T) {
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feat/x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(repo, "cmd", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := gitBranch(sub); got != "feat/x" {
		t.Fatalf("branch from subdir = %q", got)
	}

	wt := t.TempDir()
	wtGit := filepath.Join(t.TempDir(), "worktrees", "wt1")
	if err := os.MkdirAll(wtGit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtGit, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+wtGit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := gitBranch(wt); got != "main" {
		t.Fatalf("worktree branch = %q", got)
	}

	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("0123456789abcdef\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := gitBranch(repo); got != "" {
		t.Fatalf("detached HEAD should be empty, got %q", got)
	}
	if gitBranch(t.TempDir()) != "" {
		t.Fatal("non-repo should be empty")
	}
	if gitBranch("") != "" {
		t.Fatal("empty dir should be empty")
	}
}

// The built-in model patterns must match the tools' real footers (text below
// captured from live codex 0.142.5 / cursor-agent 2026.06.26 sessions), and
// the live footer must win over a stale intro header.
func TestExtractModelBuiltinPatterns(t *testing.T) {
	ms, err := adapter.LoadBuiltin()
	if err != nil {
		t.Fatal(err)
	}

	codex := "│ model:     gpt-5.5 medium   /model to change │\n" +
		"│ directory: ~/code/stroops/sloop              │\n" +
		"› Write tests for @filename\n" +
		"  gpt-5.5 medium · ~/code/stroops/sloop\n"
	if got := extractModel(codex, ms["codex"].Heuristics.Model); got != "gpt-5.5" {
		t.Fatalf("codex model = %q, want gpt-5.5", got)
	}
	// A model without a reasoning-effort suffix still matches.
	if got := extractModel("  gpt-5.5-codex · ~/x\n", ms["codex"].Heuristics.Model); got != "gpt-5.5-codex" {
		t.Fatalf("codex effortless model = %q", got)
	}

	cursor := "  → Plan, search, build anything\n" +
		"  Composer 2.5 Fast\n" +
		"  ~/code/stroops/sloop · feat/enhance-run\n"
	if got := extractModel(cursor, ms["cursor"].Heuristics.Model); got != "Composer 2.5 Fast" {
		t.Fatalf("cursor model = %q, want Composer 2.5 Fast", got)
	}

	if extractModel("no footer here", ms["codex"].Heuristics.Model) != "" {
		t.Fatal("no match must yield empty")
	}
	if extractModel("anything", "(") != "" {
		t.Fatal("bad pattern must yield empty, not panic")
	}
	if extractModel("anything", "") != "" {
		t.Fatal("empty pattern must yield empty")
	}
}

func TestTmuxStatusLabel(t *testing.T) {
	w := tmuxStatusLabel(tmux.StatusWaiting)
	if !strings.Contains(w, "waiting") || !strings.Contains(w, "fg=yellow") {
		t.Fatalf("waiting label = %q", w)
	}
	if !strings.Contains(tmuxStatusLabel(tmux.StatusWorking), "working") {
		t.Fatalf("working label")
	}
}
