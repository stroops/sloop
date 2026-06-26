package adapter

import (
	"embed"
	"io/fs"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed builtin/*.yaml
var builtinFS embed.FS

type Output struct {
	Path     string `yaml:"path"`
	Template string `yaml:"template"`
}

type Manifest struct {
	Name    string   `yaml:"name"`
	Detect  string   `yaml:"detect"`
	Launch  string   `yaml:"launch"`
	Outputs []Output `yaml:"outputs"`
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

// Render returns native-file path -> content. Only the "default" template is
// supported in Plan 1; it emits the assembled context verbatim.
func (m Manifest) Render(assembled string) map[string]string {
	out := map[string]string{}
	for _, o := range m.Outputs {
		switch o.Template {
		default: // "default" and unknown templates fall back to passthrough
			out[o.Path] = assembled
		}
	}
	return out
}
