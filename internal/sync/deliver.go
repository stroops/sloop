package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/stroops/sloop/internal/adapter"
)

type Action string

const (
	ActionCreated Action = "created"
	ActionSkipped Action = "skipped"
	ActionForeign Action = "foreign"
	ActionLinked  Action = "linked"
	ActionCopied  Action = "copied"
	ActionNone    Action = "none"
)

const agentsStarter = `# AGENTS.md

Project guidance for AI coding tools. Describe the project, conventions, and constraints here.
This is the canonical context; sloop points other tools (CLAUDE.md, GEMINI.md, ...) at this file.
`

// symlinkFunc is a seam so the copy-fallback path is testable.
var symlinkFunc = os.Symlink

func EnsureAgents(root string) (Action, error) {
	path := filepath.Join(root, "AGENTS.md")
	if _, err := os.Stat(path); err == nil {
		return ActionSkipped, nil
	} else if !os.IsNotExist(err) {
		return ActionSkipped, err
	}
	if err := os.WriteFile(path, []byte(agentsStarter), 0o644); err != nil {
		return ActionSkipped, err
	}
	return ActionCreated, nil
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
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
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
	source := filepath.Join(sloopDir, "skills")
	target := filepath.Join(root, m.Skills.Target)

	if fi, err := os.Lstat(target); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			if dst, _ := os.Readlink(target); dst == source {
				return ActionSkipped, nil
			}
		}
		return ActionForeign, nil // real file/dir or foreign symlink: leave it
	} else if !os.IsNotExist(err) {
		return ActionSkipped, err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return ActionSkipped, err
	}
	if err := symlinkFunc(source, target); err == nil {
		return ActionLinked, nil
	}
	// Fallback: copy.
	if err := copyDir(source, target); err != nil {
		return ActionSkipped, err
	}
	return ActionCopied, nil
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
			return os.MkdirAll(out, 0o755)
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
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
