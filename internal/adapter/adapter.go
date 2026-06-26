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

type Manifest struct {
	Name    string      `yaml:"name"`
	Detect  string      `yaml:"detect"`
	Launch  string      `yaml:"launch"`
	Context ContextSpec `yaml:"context"`
	Skills  SkillsSpec  `yaml:"skills"`
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

