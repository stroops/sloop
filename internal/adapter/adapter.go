package adapter

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/stroops/sloop/internal/config"
	"gopkg.in/yaml.v3"
)

//go:embed builtin/*.yaml
var builtinFS embed.FS

type ContextSpec struct {
	Mode string `yaml:"mode"` // "pointer" | "native"
	File string `yaml:"file"` // pointer mode only
}

type SkillsSpec struct {
	Target string `yaml:"target"` // dir to link .sloop/skills into; empty = none
}

// HookEvents maps each sloop status to the tool's own lifecycle event name.
// An empty event means the tool has no signal for that state.
type HookEvents struct {
	Working string `yaml:"working"` // tool started working
	Waiting string `yaml:"waiting"` // tool is blocked on the user
	Idle    string `yaml:"idle"`    // tool finished the turn
}

// HooksSpec captures how a tool exposes lifecycle hooks for precise status. It
// keeps per-provider hook knowledge in the manifest (the single provider-aware
// source), not hardcoded in commands.
type HooksSpec struct {
	Config  string     `yaml:"config"`  // config file (repo-relative or ~/…)
	Install string     `yaml:"install"` // installer strategy: "settings-json" | "" (manual)
	Docs    string     `yaml:"docs"`
	Notes   string     `yaml:"notes"`
	Events  HookEvents `yaml:"events"`
}

// HeuristicsSpec defines fallback text-matching rules to classify agent status
// when native hooks aren't available. In the future, this could be extended
// with LLM prompts for deeper "AI awareness" of the session state.
type HeuristicsSpec struct {
	Working []string `yaml:"working"`
	Waiting []string `yaml:"waiting"`
}

// RunSpec declares how `sloop run` launches a tool with an optional model and
// reasoning effort. Every field is optional: an empty flag means the CLI has no
// such knob, and sloop errors clearly if the user asks for it. sloop never
// validates the model — it forwards the string and lets the CLI accept/reject it.
type RunSpec struct {
	// Vendor is the model vendor this CLI natively serves (e.g. "anthropic").
	Vendor string `yaml:"vendor"`
	// DefaultFor lists vendors this CLI is the canonical launcher for, so a bare
	// model (`sloop run opus`) resolves to it.
	DefaultFor []string `yaml:"default_for"`
	// ModelFlag is how the CLI selects a model (e.g. "--model"); "" = unsupported.
	ModelFlag string `yaml:"model_flag"`
	// EffortFlag + EffortValues express reasoning effort: low|medium|high mapped
	// to the CLI's own token. "" flag = the CLI has no effort knob.
	EffortFlag   string            `yaml:"effort_flag"`
	EffortValues map[string]string `yaml:"effort_values"`
	// Models are the model aliases this CLI serves (completion + bare-model
	// resolution). Not exhaustive; full API ids still work via forward.
	Models []string `yaml:"models"`
}

type Manifest struct {
	Name string `yaml:"name"`
	// Detect is the binary name used to detect installation; Launch is the
	// binary to run (often the same).
	Detect  string      `yaml:"detect"`
	Launch  string      `yaml:"launch"`
	Context ContextSpec `yaml:"context"`
	Skills  SkillsSpec  `yaml:"skills"`
	Hooks   HooksSpec   `yaml:"hooks"`
	// Run declares model/effort launch knobs for `sloop run`.
	Run RunSpec `yaml:"run"`
	// Fallback text-parsing rules for status (paving the way for AI-driven awareness)
	Heuristics HeuristicsSpec `yaml:"heuristics"`
	// Scaffold lists the tool's standard project dirs that `sloop init --scaffold`
	// can create (e.g. .claude/skills, .cursor/rules), so users start from the
	// provider's expected layout instead of an ad-hoc one.
	Scaffold []string `yaml:"scaffold"`
}

func LoadBuiltin() (map[string]Manifest, error) {
	out := map[string]Manifest{}
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := fs.ReadFile(builtinFS, filepath.Join("builtin", e.Name()))
		if err != nil {
			return nil, err
		}
		var m Manifest
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		key := strings.TrimSuffix(e.Name(), ".yaml")
		out[key] = m
	}
	return out, nil
}

// Load returns built-in manifests overlaid with any user manifests in
// ~/.sloop/adapters/*.yaml (same key overrides the built-in).
func Load() (map[string]Manifest, error) {
	out, err := LoadBuiltin()
	if err != nil {
		return nil, err
	}
	dir, err := config.UserAdaptersDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var m Manifest
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		key := strings.TrimSuffix(e.Name(), ".yaml")
		out[key] = m
	}
	return out, nil
}
