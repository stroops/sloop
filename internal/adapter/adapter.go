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

type Manifest struct {
	Name string `yaml:"name"`
	// Detect is the binary name used to detect installation; Launch is the
	// binary to run (often the same).
	Detect  string      `yaml:"detect"`
	Launch  string      `yaml:"launch"`
	Context ContextSpec `yaml:"context"`
	Skills  SkillsSpec  `yaml:"skills"`
	Hooks   HooksSpec   `yaml:"hooks"`
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
