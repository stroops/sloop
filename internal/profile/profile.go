package profile

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Profile struct {
	Tool    string   `yaml:"tool"`
	Context string   `yaml:"context"` // "all" or explicit context filenames are honored in Plan 2
	Skills  []string `yaml:"skills"`
	Vault   []string `yaml:"vault"`
}

func Default(tool string) Profile {
	return Profile{Tool: tool, Context: "all"}
}

func Load(path string) (Profile, error) {
	var p Profile
	b, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	if err := yaml.Unmarshal(b, &p); err != nil {
		return p, err
	}
	return p, nil
}

func Save(path string, p Profile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
