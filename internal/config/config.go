package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const SloopDirName = ".sloop"

func GlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, SloopDirName), nil
}

func GlobalDBPath() (string, error) {
	d, err := GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "sloop.db"), nil
}

func UserAdaptersDir() (string, error) {
	d, err := GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "adapters"), nil
}

// Project is the per-project config stored at <sloopDir>/config.yaml.
type Project struct {
	Tools       []string `yaml:"tools"`
	DefaultTool string   `yaml:"default_tool"`
}

func LoadProject(sloopDir string) (*Project, error) {
	b, err := os.ReadFile(filepath.Join(sloopDir, "config.yaml"))
	if err != nil {
		return nil, err
	}
	var p Project
	if err := yaml.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func SaveProject(sloopDir string, p *Project) error {
	if err := os.MkdirAll(sloopDir, 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sloopDir, "config.yaml"), b, 0o644)
}
