package sync

import (
	"os"
	"path/filepath"
	"time"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/profile"
)

// Stale reports whether the generated native files are out of date relative to
// the canonical sources selected by the profile.
func Stale(root, sloopDir string, m adapter.Manifest, p profile.Profile) (bool, error) {
	newestSrc, err := newestSourceMtime(sloopDir, p)
	if err != nil {
		return false, err
	}
	for _, o := range m.Outputs {
		info, err := os.Stat(filepath.Join(root, o.Path))
		if os.IsNotExist(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if info.ModTime().Before(newestSrc) {
			return true, nil
		}
	}
	return false, nil
}

func newestSourceMtime(sloopDir string, p profile.Profile) (t time.Time, err error) {
	paths := []string{}
	names, err := markdownFiles(filepath.Join(sloopDir, "context"))
	if err != nil {
		return t, err
	}
	for _, n := range names {
		paths = append(paths, filepath.Join(sloopDir, "context", n))
	}
	for _, n := range p.Skills {
		paths = append(paths, filepath.Join(sloopDir, "skills", n))
	}
	for _, n := range p.Vault {
		paths = append(paths, filepath.Join(sloopDir, "vault", n))
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue // missing sources don't make output stale
		}
		if info.ModTime().After(t) {
			t = info.ModTime()
		}
	}
	return t, nil
}
