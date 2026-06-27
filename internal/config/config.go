package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const SloopDirName = ".sloop"

const (
	ModeAsk  = "ask"
	ModeAuto = "auto"
)

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

// Global is the machine-local config at ~/.sloop/config.yaml.
type Global struct {
	Mode string `yaml:"mode"`
}

func GlobalConfigPath() (string, error) {
	d, err := GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.yaml"), nil
}

func LoadGlobal() (*Global, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Global{Mode: ModeAsk}, nil
	}
	if err != nil {
		return nil, err
	}
	var g Global
	if err := yaml.Unmarshal(b, &g); err != nil {
		return nil, err
	}
	if g.Mode == "" {
		g.Mode = ModeAsk
	}
	return &g, nil
}

func SaveGlobal(g *Global) error {
	dir, err := GlobalDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yaml")
	b, err := yaml.Marshal(g)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// Project is the per-project config stored at <sloopDir>/config.yaml.
type Project struct {
	Tools       []string `yaml:"tools"`
	DefaultTool string   `yaml:"default_tool"`
	Mode        string   `yaml:"mode,omitempty"`
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
	if err := os.MkdirAll(sloopDir, 0o700); err != nil {
		return err
	}
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sloopDir, "config.yaml"), b, 0o600)
}
