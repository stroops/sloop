package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
)

type Action string

const (
	ActionCreated  Action = "created"
	ActionSkipped  Action = "skipped"
	ActionForeign  Action = "foreign"
	ActionLinked   Action = "linked"
	ActionCopied   Action = "copied"
	ActionNone     Action = "none"
	ActionBroken   Action = "broken"
	ActionRelinked Action = "relinked"
	ActionRepaired Action = "repaired"
)

// skillsPaths returns the link path, the absolute skills source, and the
// relative form used as the symlink target.
func skillsPaths(root, sloopDir, manifestTarget string) (link, source, rel string) {
	link = filepath.Join(root, manifestTarget)
	source = filepath.Join(sloopDir, "skills")
	rel, _ = filepath.Rel(filepath.Dir(link), source)
	return
}

// isOurLink reports whether a readlink destination is one sloop created:
// the canonical relative form, the legacy absolute source, or any
// "<...>/.sloop/skills" path.
func isOurLink(dst, source, rel string) bool {
	return dst == rel || dst == source ||
		(filepath.Base(dst) == "skills" && filepath.Base(filepath.Dir(dst)) == config.SloopDirName)
}

const agentsStarter = `# AGENTS.md

Project guidance for AI coding tools. Describe the project, conventions, and constraints here.
This is the canonical context; sloop points other tools (CLAUDE.md, GEMINI.md, ...) at this file.
`

// symlinkFunc is a seam so the copy-fallback path is testable.
var symlinkFunc = os.Symlink

// EnsureAgentsContent writes content to AGENTS.md if it is missing; it never
// overwrites an existing one (create-if-missing).
func EnsureAgentsContent(root, content string) (Action, error) {
	path := filepath.Join(root, "AGENTS.md")
	if _, err := os.Stat(path); err == nil {
		return ActionSkipped, nil
	} else if !os.IsNotExist(err) {
		return ActionSkipped, err
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return ActionSkipped, err
	}
	return ActionCreated, nil
}

func EnsureAgents(root string) (Action, error) {
	return EnsureAgentsContent(root, agentsStarter)
}

func PointerContent(toolName, file string) string {
	return fmt.Sprintf(`# %s

This file provides guidance to %s when working with code in this repository.

**Note**: This project uses AGENTS.md for detailed guidance.

## Primary Reference

See `+"`AGENTS.md`"+` in this same directory for the main project documentation and guidance.
`, file, toolName)
}

func SyncContext(root string, m adapter.Manifest) (Action, error) {
	if m.Context.Mode != "pointer" { // "native" (or unset): nothing to generate
		return ActionSkipped, nil
	}
	path := filepath.Join(root, m.Context.File)
	want := PointerContent(m.Name, m.Context.File)
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
			return ActionSkipped, err
		}
		return ActionCreated, nil
	}
	if err != nil {
		return ActionSkipped, err
	}
	if string(existing) == want {
		return ActionSkipped, nil
	}
	return ActionForeign, nil
}

func SyncSkills(root, sloopDir string, m adapter.Manifest) (Action, error) {
	if m.Skills.Target == "" {
		return ActionNone, nil
	}
	link, source, rel := skillsPaths(root, sloopDir, m.Skills.Target)

	if fi, err := os.Lstat(link); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			dst, _ := os.Readlink(link)
			_, statErr := os.Stat(link) // resolves through the link
			switch {
			case dst == rel && statErr == nil:
				return ActionSkipped, nil
			case isOurLink(dst, source, rel) && statErr == nil:
				if err := relink(link, rel); err != nil {
					return ActionSkipped, err
				}
				return ActionRelinked, nil
			case isOurLink(dst, source, rel):
				return ActionBroken, nil // our link, destination gone
			}
		}
		return ActionForeign, nil // real dir/file or a foreign symlink
	} else if !os.IsNotExist(err) {
		return ActionSkipped, err
	}

	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		return ActionSkipped, err
	}
	if err := symlinkFunc(rel, link); err == nil {
		return ActionLinked, nil
	}
	if err := copyDir(source, link); err != nil {
		return ActionSkipped, err
	}
	return ActionCopied, nil
}

func relink(link, rel string) error {
	if err := os.Remove(link); err != nil {
		return err
	}
	return symlinkFunc(rel, link)
}

// backupAside renames an occupant to "<name>.sloopbak-<timestamp>" (never deletes).
func backupAside(path string) error {
	bak := fmt.Sprintf("%s.sloopbak-%s", path, time.Now().Format("20060102-150405"))
	return os.Rename(path, bak)
}

func RepairContext(root string, m adapter.Manifest) (Action, error) {
	if m.Context.Mode != "pointer" {
		return ActionSkipped, nil
	}
	switch a, err := SyncContext(root, m); {
	case err != nil:
		return ActionSkipped, err
	case a != ActionForeign: // created/skipped already correct → nothing to repair
		return a, nil
	}
	path := filepath.Join(root, m.Context.File)
	if err := backupAside(path); err != nil {
		return ActionSkipped, err
	}
	if _, err := SyncContext(root, m); err != nil { // now writes the pointer (missing)
		return ActionSkipped, err
	}
	return ActionRepaired, nil
}

func RepairSkills(root, sloopDir string, m adapter.Manifest) (Action, error) {
	switch a, err := SyncSkills(root, sloopDir, m); {
	case err != nil:
		return ActionSkipped, err
	case a != ActionForeign && a != ActionBroken:
		return a, nil // none/linked/relinked/created/copied → already handled
	}
	link, _, _ := skillsPaths(root, sloopDir, m.Skills.Target)
	if err := backupAside(link); err != nil {
		return ActionSkipped, err
	}
	if _, err := SyncSkills(root, sloopDir, m); err != nil {
		return ActionSkipped, err
	}
	return ActionRepaired, nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(out, 0o700)
		}
		return copyFile(p, out)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
