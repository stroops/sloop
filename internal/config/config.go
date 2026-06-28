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

// ConfigVersion is the current config schema version, written to new config
// files so future schema changes can migrate safely.
const ConfigVersion = 1

// Profile is a named launch variant of a provider: the same tool with extra
// env (e.g. CLAUDE_CONFIG_DIR to select a second account). Invoked as
// `sloop run @<name>`; the profile name becomes the session's instance suffix.
type Profile struct {
	Tool string            `yaml:"tool"`
	Env  map[string]string `yaml:"env,omitempty"`
}

// Global is the machine-local config at ~/.sloop/config.yaml.
type Global struct {
	Version int    `yaml:"version"`
	Mode    string `yaml:"mode"`
	// Lang is the preferred UI language for hints (e.g. "en", "vi"); empty =
	// auto from $SLOOP_LANG/$LANG. Hints nil = enabled, false = off.
	Lang  string `yaml:"lang,omitempty"`
	Hints *bool  `yaml:"hints,omitempty"`
	// Profiles are reusable launch variants, keyed by name.
	Profiles map[string]Profile `yaml:"profiles,omitempty"`
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
		return &Global{Version: ConfigVersion, Mode: ModeAsk}, nil
	}
	if err != nil {
		return nil, err
	}
	var g Global
	if err := yaml.Unmarshal(b, &g); err != nil {
		return nil, err
	}
	if g.Version == 0 {
		g.Version = ConfigVersion
	}
	if g.Mode == "" {
		g.Mode = ModeAsk
	}
	return &g, nil
}

func SaveGlobal(g *Global) error {
	if g.Version == 0 {
		g.Version = ConfigVersion
	}
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
	Version     int      `yaml:"version"`
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
	if p.Version == 0 {
		p.Version = ConfigVersion
	}
	return &p, nil
}

func SaveProject(sloopDir string, p *Project) error {
	if p.Version == 0 {
		p.Version = ConfigVersion
	}
	if err := os.MkdirAll(sloopDir, 0o700); err != nil {
		return err
	}
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sloopDir, "config.yaml"), b, 0o600)
}
