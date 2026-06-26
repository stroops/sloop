package workspace

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/stroops/sloop/internal/config"
)

var ErrNotFound = errors.New("no .sloop workspace found")

type Workspace struct {
	Name string
	Root string
}

func (w Workspace) SloopDir() string {
	return filepath.Join(w.Root, config.SloopDirName)
}

func Resolve(startDir string) (*Workspace, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}
	for {
		candidate := filepath.Join(dir, config.SloopDirName)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return &Workspace{Name: filepath.Base(dir), Root: dir}, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, ErrNotFound
		}
		dir = parent
	}
}
