package sync

import (
	"os"
	"path/filepath"

	"github.com/stroops/sloop/internal/adapter"
)

func AgentsState(root string) string {
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err == nil {
		return "ok"
	}
	return "missing"
}

func ContextState(root string, m adapter.Manifest) string {
	if m.Context.Mode != "pointer" {
		return "native"
	}
	path := filepath.Join(root, m.Context.File)
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "missing"
	}
	if err != nil {
		return "missing"
	}
	if string(existing) == PointerContent(m.Name, m.Context.File) {
		return "ok"
	}
	return "foreign"
}

func SkillsState(root, sloopDir string, m adapter.Manifest) string {
	if m.Skills.Target == "" {
		return "none"
	}
	link, source, rel := skillsPaths(root, sloopDir, m.Skills.Target)
	fi, err := os.Lstat(link)
	if err != nil {
		return "missing"
	}
	if fi.Mode()&os.ModeSymlink != 0 && isOurLink(readlink(link), source, rel) {
		if _, err := os.Stat(link); err == nil {
			return "linked"
		}
		return "broken"
	}
	return "present"
}

func readlink(p string) string { d, _ := os.Readlink(p); return d }
