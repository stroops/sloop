package commands

import (
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
)

func loadManifestsForTest(t *testing.T) map[string]adapter.Manifest {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // hermetic: ignore the user's ~/.sloop/adapters
	m, err := adapter.Load()
	if err != nil {
		t.Fatalf("adapter.Load: %v", err)
	}
	return m
}

func TestPlanLaunch(t *testing.T) {
	m := loadManifestsForTest(t)
	cases := []struct {
		name                              string
		target, provider, model, effort   string
		wantKey, wantModel, wantErrSubstr string
	}{
		{name: "tool key", target: "claude", wantKey: "claude"},
		{name: "binary alias", target: "agent", wantKey: "cursor"},
		{name: "bare model -> home CLI", target: "opus", wantKey: "claude", wantModel: "opus"},
		{name: "tool + model flag", target: "claude", model: "sonnet", wantKey: "claude", wantModel: "sonnet"},
		{name: "provider flag + model", provider: "cursor", model: "opus", wantKey: "cursor", wantModel: "opus"},
		{name: "default tool when empty", target: "", wantKey: "claude"},
		{name: "unknown token", target: "nope", wantErrSubstr: "unknown tool or model"},
		{name: "conflict tool vs provider", target: "claude", provider: "codex", wantErrSubstr: "conflicting"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := planLaunch(c.target, c.provider, c.model, c.effort, "claude", m)
			if c.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErrSubstr) {
					t.Fatalf("want error containing %q, got %v", c.wantErrSubstr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("planLaunch: %v", err)
			}
			if p.toolKey != c.wantKey {
				t.Fatalf("toolKey = %q, want %q", p.toolKey, c.wantKey)
			}
			if p.model != c.wantModel {
				t.Fatalf("model = %q, want %q", p.model, c.wantModel)
			}
		})
	}
}

func TestBuildRunArgs(t *testing.T) {
	claude := adapter.Manifest{Name: "Claude", Run: adapter.RunSpec{ModelFlag: "--model", Prompt: "positional"}}
	codex := adapter.Manifest{Name: "Codex", Run: adapter.RunSpec{
		ModelFlag:    "--model",
		EffortFlag:   "-c",
		EffortValues: map[string]string{"high": "model_reasoning_effort=high"},
	}}
	noKnobs := adapter.Manifest{Name: "Agy"}

	t.Run("model forwarded", func(t *testing.T) {
		got, err := buildRunArgs(claude, "opus", "", "", []string{"--foo"})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"--model", "opus", "--foo"}
		if !eqStrs(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})
	t.Run("effort forwarded", func(t *testing.T) {
		got, err := buildRunArgs(codex, "", "high", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"-c", "model_reasoning_effort=high"}
		if !eqStrs(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})
	t.Run("task seeds positional after model", func(t *testing.T) {
		got, err := buildRunArgs(claude, "opus", "", "fix the auth bug", nil)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"--model", "opus", "fix the auth bug"}
		if !eqStrs(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})
	t.Run("task unsupported errors", func(t *testing.T) {
		if _, err := buildRunArgs(codex, "", "", "do a thing", nil); err == nil {
			t.Fatal("want error for task on a CLI with no prompt support")
		}
	})
	t.Run("model unsupported errors", func(t *testing.T) {
		if _, err := buildRunArgs(noKnobs, "opus", "", "", nil); err == nil {
			t.Fatal("want error for model on a CLI with no model_flag")
		}
	})
	t.Run("effort unsupported errors", func(t *testing.T) {
		if _, err := buildRunArgs(claude, "", "high", "", nil); err == nil {
			t.Fatal("want error for effort on a CLI with no effort_flag")
		}
	})
	t.Run("bad effort value errors", func(t *testing.T) {
		if _, err := buildRunArgs(codex, "", "turbo", "", nil); err == nil {
			t.Fatal("want error for effort not in low|medium|high")
		}
	})
	t.Run("passthrough only", func(t *testing.T) {
		got, err := buildRunArgs(noKnobs, "", "", "", []string{"--help"})
		if err != nil || !eqStrs(got, []string{"--help"}) {
			t.Fatalf("got %v err %v", got, err)
		}
	})
}

func eqStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
