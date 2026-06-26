# Sloop Sync v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the v1 "full-copy passthrough from `.sloop/context/`" sync with the Model-B delivery model: `AGENTS.md` is canonical, sloop generates thin pointer files (create-if-missing, never overwrite) for own-file tools and symlinks `.sloop/skills/` into each tool's native skills dir.

**Architecture:** Adapter manifests gain `context{mode,file}` + `skills{target}` (replacing `outputs`). A new `internal/sync` delivery layer ensures `AGENTS.md`, writes pointer files only when missing/identical, and symlinks skills (copy fallback). Commands (`init`/`sync`/`run`/`status`) rewire to it; old context-assembly code is removed. The work is sequenced additive-first so each task builds and passes tests.

**Tech Stack:** Go 1.26, Cobra, `gopkg.in/yaml.v3`, `go:embed`, `os` symlink/file APIs.

## Global Constraints

- Spec of record: `docs/superpowers/specs/2026-06-26-sloop-sync-v2-design.md`.
- Canonical: `AGENTS.md` (repo root, context); `.sloop/skills/*.md` (skills); `.sloop/vault/` (NOT delivered).
- **No `.sloop/context/`** after this change.
- Delivery philosophy: **create-if-missing, never overwrite, no markers, no `--force`.** Existing-but-different files are left untouched with a warning.
- Pointer files are for `context.mode: pointer` tools only; `native` tools read `AGENTS.md` directly (sloop generates nothing for them beyond ensuring `AGENTS.md` exists).
- Skills delivery: symlink **at** `<root>/<skills.target>` **pointing to** `.sloop/skills/`; fall back to copy when symlink fails. Adapters with empty `skills.target` deliver nothing.
- Built-in `skills.target`: only `claude` = `.claude/skills`; others empty (best-guess, editable).
- Pre-release: **no migration** — old workspaces are recreated with `sloop init`.
- No new Go modules; do **not** run `go mod tidy`. Use `go build ./...` + focused `go test`.
- Commit subjects: `type: Capitalized subject` (type ∈ feat/fix/refactor/docs/chore/perf/revert), no trailing period, no `Co-Authored-By` trailer.
- Every task must leave `go build ./...` and `go test ./...` green.

---

### Task 1: Adapter manifest v2 fields (additive) + rewrite built-in YAMLs

**Files:**
- Modify: `internal/adapter/adapter.go` (add `ContextSpec`, `SkillsSpec`, and `Context`/`Skills` fields on `Manifest`; keep `Output`/`Outputs`/`Render` for now)
- Modify: `internal/adapter/builtin/{claude,cursor,codex,copilot,gemini}.yaml` (add `context:`/`skills:` blocks; keep existing `outputs:`)
- Test: `internal/adapter/v2_test.go`

**Interfaces:**
- Consumes: existing `Manifest`, `LoadBuiltin`.
- Produces:
  - `type ContextSpec struct { Mode, File string }` (yaml: `mode`, `file`)
  - `type SkillsSpec struct { Target string }` (yaml: `target`)
  - `Manifest.Context ContextSpec` (yaml: `context`), `Manifest.Skills SkillsSpec` (yaml: `skills`)

- [ ] **Step 1: Write the failing test**

```go
package adapter

import "testing"

func TestManifestV2Fields(t *testing.T) {
	m, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin: %v", err)
	}
	if m["claude"].Context.Mode != "pointer" || m["claude"].Context.File != "CLAUDE.md" {
		t.Fatalf("claude context = %+v", m["claude"].Context)
	}
	if m["claude"].Skills.Target != ".claude/skills" {
		t.Fatalf("claude skills = %+v", m["claude"].Skills)
	}
	if m["gemini"].Context.Mode != "pointer" || m["gemini"].Context.File != "GEMINI.md" {
		t.Fatalf("gemini context = %+v", m["gemini"].Context)
	}
	for _, native := range []string{"cursor", "codex", "copilot"} {
		if m[native].Context.Mode != "native" {
			t.Fatalf("%s should be native, got %q", native, m[native].Context.Mode)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/ -run TestManifestV2Fields -v`
Expected: FAIL (Context/Skills fields are zero values).

- [ ] **Step 3: Add the struct fields**

In `internal/adapter/adapter.go`, add the two specs and extend `Manifest` (keep the existing `Output`, `Outputs`, and `Render` exactly as they are):

```go
type ContextSpec struct {
	Mode string `yaml:"mode"` // "pointer" | "native"
	File string `yaml:"file"` // pointer mode only
}

type SkillsSpec struct {
	Target string `yaml:"target"` // dir to link .sloop/skills into; empty = none
}
```

And add to the `Manifest` struct:
```go
	Context ContextSpec `yaml:"context"`
	Skills  SkillsSpec  `yaml:"skills"`
```

- [ ] **Step 4: Update the built-in YAMLs (add blocks, keep `outputs:`)**

`claude.yaml`:
```yaml
name: Claude Code
detect: claude
launch: claude
outputs:
  - path: CLAUDE.md
    template: default
context:
  mode: pointer
  file: CLAUDE.md
skills:
  target: .claude/skills
```

`gemini.yaml`:
```yaml
name: Gemini CLI
detect: gemini
launch: gemini
outputs:
  - path: GEMINI.md
    template: default
context:
  mode: pointer
  file: GEMINI.md
skills:
  target: ""
```

`cursor.yaml`:
```yaml
name: Cursor CLI
detect: agent
launch: agent
outputs:
  - path: AGENTS.md
    template: default
context:
  mode: native
  file: ""
skills:
  target: ""
```

`codex.yaml`:
```yaml
name: OpenAI Codex CLI
detect: codex
launch: codex
outputs:
  - path: AGENTS.md
    template: default
context:
  mode: native
  file: ""
skills:
  target: ""
```

`copilot.yaml`:
```yaml
name: GitHub Copilot CLI
detect: copilot
launch: copilot
outputs:
  - path: AGENTS.md
    template: default
context:
  mode: native
  file: ""
skills:
  target: ""
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/adapter/ -v && go build ./...`
Expected: PASS (new test + all existing adapter tests still green).

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/
git commit -m "feat: Add v2 context and skills fields to adapter manifests"
```

---

### Task 2: Sync delivery layer (pointer + symlink + copy fallback)

**Files:**
- Create: `internal/sync/deliver.go`
- Test: `internal/sync/deliver_test.go`

**Interfaces:**
- Consumes: `adapter.Manifest`, `adapter.ContextSpec`, `adapter.SkillsSpec`.
- Produces:
  - `type Action string` with `ActionCreated`, `ActionSkipped`, `ActionForeign`, `ActionLinked`, `ActionCopied`, `ActionNone`
  - `func EnsureAgents(root string) (Action, error)` — write starter `AGENTS.md` if missing
  - `func PointerContent(toolName, file string) string`
  - `func SyncContext(root string, m adapter.Manifest) (Action, error)`
  - `func SyncSkills(root, sloopDir string, m adapter.Manifest) (Action, error)`
  - `var symlinkFunc = os.Symlink` (overridable seam for the copy-fallback test)

- [ ] **Step 1: Write the failing tests**

```go
package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
)

func TestEnsureAgentsCreatesThenSkips(t *testing.T) {
	root := t.TempDir()
	a, err := EnsureAgents(root)
	if err != nil || a != ActionCreated {
		t.Fatalf("first EnsureAgents = %v, %v", a, err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("AGENTS.md not created: %v", err)
	}
	a, _ = EnsureAgents(root)
	if a != ActionSkipped {
		t.Fatalf("second EnsureAgents = %v, want skipped", a)
	}
}

func TestSyncContextPointerCreateIdempotentForeign(t *testing.T) {
	root := t.TempDir()
	m := adapter.Manifest{Name: "Claude Code", Context: adapter.ContextSpec{Mode: "pointer", File: "CLAUDE.md"}}

	a, err := SyncContext(root, m)
	if err != nil || a != ActionCreated {
		t.Fatalf("create = %v, %v", a, err)
	}
	a, _ = SyncContext(root, m)
	if a != ActionSkipped {
		t.Fatalf("idempotent = %v, want skipped", a)
	}
	// User edits it → foreign, left untouched.
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("MY OWN CONTENT"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ = SyncContext(root, m)
	if a != ActionForeign {
		t.Fatalf("foreign = %v, want foreign", a)
	}
	b, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if string(b) != "MY OWN CONTENT" {
		t.Fatalf("foreign file was overwritten: %q", string(b))
	}
}

func TestSyncContextNativeDoesNothing(t *testing.T) {
	root := t.TempDir()
	m := adapter.Manifest{Context: adapter.ContextSpec{Mode: "native"}}
	a, err := SyncContext(root, m)
	if err != nil || a != ActionSkipped {
		t.Fatalf("native = %v, %v", a, err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("native SyncContext must not create files")
	}
}

func TestSyncSkillsSymlinkThenIdempotent(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}

	a, err := SyncSkills(root, sloopDir, m)
	if err != nil || a != ActionLinked {
		t.Fatalf("link = %v, %v", a, err)
	}
	fi, err := os.Lstat(filepath.Join(root, ".claude", "skills"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink at .claude/skills: %v", err)
	}
	a, _ = SyncSkills(root, sloopDir, m)
	if a != ActionSkipped {
		t.Fatalf("idempotent skills = %v, want skipped", a)
	}
}

func TestSyncSkillsCopyFallback(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sloopDir, "skills", "review.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Force symlink to fail.
	orig := symlinkFunc
	symlinkFunc = func(string, string) error { return os.ErrPermission }
	defer func() { symlinkFunc = orig }()

	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}
	a, err := SyncSkills(root, sloopDir, m)
	if err != nil || a != ActionCopied {
		t.Fatalf("copy fallback = %v, %v", a, err)
	}
	b, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "review.md"))
	if err != nil || string(b) != "hi" {
		t.Fatalf("copied skill missing/wrong: %q %v", string(b), err)
	}
}

func TestSyncSkillsNoTarget(t *testing.T) {
	a, err := SyncSkills(t.TempDir(), t.TempDir(), adapter.Manifest{})
	if err != nil || a != ActionNone {
		t.Fatalf("no target = %v, %v", a, err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sync/ -run 'TestEnsureAgents|TestSyncContext|TestSyncSkills' -v`
Expected: FAIL (undefined: EnsureAgents / SyncContext / SyncSkills / Action*).

- [ ] **Step 3: Write the implementation**

`internal/sync/deliver.go`:
```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sync/ -v && go build ./...`
Expected: PASS (new delivery tests + existing Plan-1 sync tests still green).

- [ ] **Step 5: Commit**

```bash
git add internal/sync/deliver.go internal/sync/deliver_test.go
git commit -m "feat: Add v2 sync delivery (pointer create-if-missing and skills symlink)"
```

---

### Task 3: Rewire init/sync/run/status commands to v2 delivery

**Files:**
- Modify: `internal/cli/commands/init.go` (create `AGENTS.md` via `EnsureAgents`; drop `context/` creation; keep skills/vault/profiles/config/registration)
- Modify: `internal/cli/commands/sync.go` (`RunSync` uses `EnsureAgents`+`SyncContext`+`SyncSkills`)
- Modify: `internal/cli/commands/status.go` (`RunStatus` reports agents/ctx/skills state)
- Create: `internal/sync/state.go` (read-only state helpers for status)
- Test: `internal/sync/state_test.go`; update `internal/cli/commands/initcmd_test.go`, `synccmd_test.go`, `statusls_test.go`

**Interfaces:**
- Consumes: `sync.EnsureAgents/SyncContext/SyncSkills`, `adapter.Load`, `config.LoadProject`, `workspace.Resolve`, `resolveProfile`.
- Produces:
  - `func ContextState(root string, m adapter.Manifest) string` → `native|ok|missing|foreign`
  - `func SkillsState(root, sloopDir string, m adapter.Manifest) string` → `none|linked|present|missing`
  - `func AgentsState(root string) string` → `ok|missing`
  - `RunSync(startDir, target string) ([]string, error)` (same signature; new internals, returns action log lines)

- [ ] **Step 1: Write failing state-helper tests**

`internal/sync/state_test.go`:
```go
package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
)

func TestContextStateTransitions(t *testing.T) {
	root := t.TempDir()
	m := adapter.Manifest{Name: "Claude Code", Context: adapter.ContextSpec{Mode: "pointer", File: "CLAUDE.md"}}
	if got := ContextState(root, m); got != "missing" {
		t.Fatalf("want missing, got %s", got)
	}
	if _, err := SyncContext(root, m); err != nil {
		t.Fatal(err)
	}
	if got := ContextState(root, m); got != "ok" {
		t.Fatalf("want ok, got %s", got)
	}
	if got := ContextState(root, adapter.Manifest{Context: adapter.ContextSpec{Mode: "native"}}); got != "native" {
		t.Fatalf("want native, got %s", got)
	}
}

func TestSkillsStateLinked(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}
	if got := SkillsState(root, sloopDir, m); got != "missing" {
		t.Fatalf("want missing, got %s", got)
	}
	if _, err := SyncSkills(root, sloopDir, m); err != nil {
		t.Fatal(err)
	}
	if got := SkillsState(root, sloopDir, m); got != "linked" {
		t.Fatalf("want linked, got %s", got)
	}
	if got := SkillsState(root, sloopDir, adapter.Manifest{}); got != "none" {
		t.Fatalf("want none, got %s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sync/ -run 'TestContextState|TestSkillsState' -v`
Expected: FAIL (undefined: ContextState / SkillsState).

- [ ] **Step 3: Add the read-only state helpers**

`internal/sync/state.go`:
```go
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
	target := filepath.Join(root, m.Skills.Target)
	fi, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return "missing"
	}
	if err != nil {
		return "missing"
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		if dst, _ := os.Readlink(target); dst == filepath.Join(sloopDir, "skills") {
			return "linked"
		}
	}
	return "present"
}
```

- [ ] **Step 4: Run helper tests**

Run: `go test ./internal/sync/ -run 'TestContextState|TestSkillsState' -v`
Expected: PASS.

- [ ] **Step 5: Rewrite `RunSync` (sync.go)**

Replace the body of `RunSync` in `internal/cli/commands/sync.go` (keep `resolveProfile` and the cobra command wiring; only the core changes). New `RunSync`:
```go
func RunSync(startDir, target string) ([]string, error) {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return nil, err
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return nil, err
	}
	prof, err := resolveProfile(ws.SloopDir(), target, proj.DefaultTool)
	if err != nil {
		return nil, err
	}
	manifests, err := adapter.Load()
	if err != nil {
		return nil, err
	}
	m, ok := manifests[prof.Tool]
	if !ok {
		return nil, fmt.Errorf("unknown tool %q (no adapter)", prof.Tool)
	}

	var log []string
	if a, err := syncpkg.EnsureAgents(ws.Root); err != nil {
		return nil, err
	} else if a == syncpkg.ActionCreated {
		log = append(log, "created AGENTS.md")
	}
	switch a, err := syncpkg.SyncContext(ws.Root, m); {
	case err != nil:
		return nil, err
	case a == syncpkg.ActionCreated:
		log = append(log, "created "+m.Context.File)
	case a == syncpkg.ActionForeign:
		log = append(log, m.Context.File+" exists, left as-is")
	}
	switch a, err := syncpkg.SyncSkills(ws.Root, ws.SloopDir(), m); {
	case err != nil:
		return nil, err
	case a == syncpkg.ActionLinked:
		log = append(log, "linked "+m.Skills.Target)
	case a == syncpkg.ActionCopied:
		log = append(log, "copied skills to "+m.Skills.Target)
	case a == syncpkg.ActionForeign:
		log = append(log, m.Skills.Target+" exists, left as-is")
	}
	return log, nil
}
```

Remove now-unused imports in sync.go (`profile` is still used by `resolveProfile`; `os`/`filepath` still used by `resolveProfile`). Keep `syncpkg` alias import.

- [ ] **Step 6: Rewrite `RunInit` (init.go)**

In `internal/cli/commands/init.go`, replace the `context/project.md` creation with an `AGENTS.md` starter, and drop `context` from the created subdirs:
- Change the subdir loop from `{"context", "skills", "vault", "profiles"}` to `{"skills", "vault", "profiles"}`.
- Remove the `os.WriteFile(.../context/project.md, starterContext, ...)` block and the `starterContext` constant.
- After creating dirs, call `syncpkg.EnsureAgents(dir)` (import the sync package as `syncpkg`):
```go
	if _, err := syncpkg.EnsureAgents(dir); err != nil {
		return err
	}
```
Keep detection-driven tool enablement, profile creation, `.gitignore`, and workspace registration unchanged.

- [ ] **Step 7: Rewrite `RunStatus` (status.go)**

Replace the `sync:` field with the v2 delivery state. New `RunStatus` body:
```go
func RunStatus(startDir string, w io.Writer) error {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return err
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return err
	}
	tool := proj.DefaultTool
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	m := manifests[tool]
	fmt.Fprintf(w, "⚓ %s · %s · agents:%s · ctx:%s · skills:%s\n",
		ws.Name, tool,
		syncpkg.AgentsState(ws.Root),
		syncpkg.ContextState(ws.Root, m),
		syncpkg.SkillsState(ws.Root, ws.SloopDir(), m),
	)
	return nil
}
```
Adjust imports: status.go now needs `syncpkg "github.com/stroops/sloop/internal/sync"` and `adapter`; it no longer needs `profile` if `resolveProfile` was the only user (keep whatever the build requires — run `go build` to confirm).

- [ ] **Step 8: Update command tests**

In `initcmd_test.go` `TestRunInitScaffolds`: replace the `.sloop/context/project.md` assertion with assertions that `AGENTS.md` exists at `dir` and that `.sloop/context` does **not** exist:
```go
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sloop", "context")); !os.IsNotExist(err) {
		t.Fatalf(".sloop/context should not exist")
	}
```
(Keep the `t.Setenv("PATH", t.TempDir())` hermeticity line and the profiles/.gitignore/skills/vault assertions.)

In `synccmd_test.go` `TestRunSyncWritesClaudeMd`: the marker line is no longer copied into CLAUDE.md (it's now a pointer). Replace the assertion that CLAUDE.md contains `MARKER-CONTEXT` with: CLAUDE.md exists and contains `AGENTS.md` (the pointer reference):
```go
	written, err := RunSync(dir, "claude")
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(b), "AGENTS.md") {
		t.Fatalf("CLAUDE.md should point to AGENTS.md:\n%s", string(b))
	}
	_ = written
```
(Drop the `MARKER-CONTEXT` write and the `strings`-based content check of assembled context. Keep `strings` import for the new `Contains`.)

In `statusls_test.go` `TestRunStatusShowsWorkspaceAndStale`: replace the `sync:` substring check with `skills:` (and keep the workspace-name check):
```go
	if !strings.Contains(out, filepathBase(dir)) || !strings.Contains(out, "skills:") {
		t.Fatalf("status missing workspace/skills:\n%s", out)
	}
```

- [ ] **Step 9: Run the full suite + build**

Run: `go build ./... && go test ./...`
Expected: build clean, all packages PASS.

- [ ] **Step 10: End-to-end smoke**

Run:
```bash
go build -o /tmp/sloop ./cmd/sloop
export HOME="$(mktemp -d)"
WORK="$(mktemp -d)/svc" && mkdir -p "$WORK" && cd "$WORK"
/tmp/sloop init
test -f AGENTS.md && echo "AGENTS.md created"
test ! -d .sloop/context && echo "no .sloop/context"
/tmp/sloop sync claude
echo "--- CLAUDE.md ---"; cat CLAUDE.md
ls -la .claude/skills
/tmp/sloop status
/tmp/sloop sync claude   # second run should be a no-op
```
Expected: `AGENTS.md` created; no `.sloop/context`; `CLAUDE.md` is a pointer to AGENTS.md; `.claude/skills` is a symlink; status shows `agents:ok ctx:ok skills:linked`; second sync prints nothing new.

- [ ] **Step 11: Commit**

```bash
git add internal/cli/commands/ internal/sync/state.go internal/sync/state_test.go
git commit -m "feat: Rewire init sync run status to v2 delivery"
```

---

### Task 4: Remove dead v1 code and simplify Profile

**Files:**
- Modify: `internal/adapter/adapter.go` (remove `Output`, `Outputs` field, `Render`)
- Modify: `internal/adapter/builtin/*.yaml` (remove `outputs:` blocks)
- Modify: `internal/adapter/adapter_test.go`, `builtins_test.go` (drop `Outputs`/`Render` assertions)
- Modify: `internal/sync/sync.go` (remove `Assemble`, `WriteNativeFiles`); delete `internal/sync/freshness.go` + `freshness_test.go`; update `internal/sync/sync_test.go`
- Modify: `internal/profile/profile.go` (Profile = `{Tool}`); update `internal/profile/profile_test.go`

**Interfaces:**
- Produces: `type Profile struct { Tool string }`; `func Default(tool string) Profile` → `{Tool: tool}`.

- [ ] **Step 1: Simplify Profile**

In `internal/profile/profile.go`, reduce the struct and `Default`:
```go
type Profile struct {
	Tool string `yaml:"tool"`
}

func Default(tool string) Profile {
	return Profile{Tool: tool}
}
```
Keep `Load`/`Save` as-is. In `internal/profile/profile_test.go`, update `TestDefault` (drop the `Context == "all"` check) and `TestSaveLoadRoundtrip` (drop `Skills` from the round-trip; assert only `Tool`):
```go
func TestDefault(t *testing.T) {
	if Default("claude").Tool != "claude" {
		t.Fatal("default tool wrong")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claude.yaml")
	if err := Save(path, Profile{Tool: "claude"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Tool != "claude" {
		t.Fatalf("roundtrip: %+v", got)
	}
}
```

- [ ] **Step 2: Remove v1 sync functions**

In `internal/sync/sync.go`, delete `Assemble`, `WriteNativeFiles`, and the now-unused helpers `markdownFiles`/`appendFile` **only if** they are unused after deletion (note: `newestSourceMtime` in `freshness.go` used `markdownFiles`; that file is being deleted too, so `markdownFiles` becomes unused — remove it). Delete `internal/sync/freshness.go` and `internal/sync/freshness_test.go`:
```bash
git rm internal/sync/freshness.go internal/sync/freshness_test.go
```
In `internal/sync/sync_test.go`, remove `TestAssembleOrdersContextThenSkills` and `TestWriteNativeFiles` (the functions they test are gone). Keep `mustWrite` only if another test in the package still uses it; if nothing uses it, remove it to avoid an unused-function compile error. (The deliver/state tests define their own helpers, so `mustWrite` is likely now unused — remove it.)

- [ ] **Step 3: Remove v1 adapter Output/Render**

In `internal/adapter/adapter.go`, delete the `Output` type, the `Outputs []Output` field, and the `Render` method. Remove `outputs:` from all five `internal/adapter/builtin/*.yaml`. In `internal/adapter/adapter_test.go` remove `TestRenderDefaultTemplate` and drop the `Outputs` assertions from `TestLoadBuiltinClaude` (keep the launch/detect checks). In `internal/adapter/builtins_test.go`, remove the `output` expectations from the table (keep `launch` and the key-presence checks).

- [ ] **Step 4: Build and run the full suite**

Run: `go build ./... && go test ./...`
Expected: build clean (no unused-symbol errors), all packages PASS. If the compiler reports an unused import/function, remove it.

- [ ] **Step 5: End-to-end smoke (regression)**

Run:
```bash
go build -o /tmp/sloop ./cmd/sloop
export HOME="$(mktemp -d)"
WORK="$(mktemp -d)/svc" && mkdir -p "$WORK" && cd "$WORK"
/tmp/sloop init && /tmp/sloop sync claude && /tmp/sloop status && /tmp/sloop tools
```
Expected: init creates AGENTS.md; sync creates CLAUDE.md pointer + `.claude/skills` symlink; status shows `agents:ok ctx:ok skills:linked`; tools still lists all five adapters.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: Remove v1 context-copy code and simplify profile"
```

---

## Self-Review

**Spec coverage:**
- §2 canonical sources (AGENTS.md, .sloop/skills, vault not delivered) → Tasks 2,3 ✓
- §2 drop `.sloop/context/` → Task 3 (init) ✓
- §3.1 pointer create-if-missing/idempotent/foreign → Task 2 (`SyncContext`) ✓
- §3.1 native mode generates nothing → Task 2 ✓
- §3.2 skills symlink + copy fallback + create-if-missing → Task 2 (`SyncSkills`) ✓
- §3.3 vault not delivered → no delivery code references vault ✓
- §4 adapter manifest v2 (`context`/`skills`) + built-ins → Task 1 (add), Task 4 (remove old) ✓
- §5 init AGENTS.md starter; sync flow; status line → Task 3 ✓
- §6 profile simplification → Task 4 ✓
- §7 code migration / removal of Assemble/WriteNativeFiles/freshness/Render/Outputs → Task 4 ✓
- §8 error handling (foreign→warn, symlink→copy fallback, unknown tool→error) → Tasks 2,3 ✓
- §9 testing → each task's TDD steps + e2e smokes ✓

**Placeholder scan:** No TBD/TODO. `skills.target: ""` for non-claude tools is the spec-mandated best-guess-empty value, not a placeholder.

**Type consistency:** `adapter.ContextSpec{Mode,File}`, `adapter.SkillsSpec{Target}`, `sync.Action` constants, `EnsureAgents/SyncContext/SyncSkills(root[,sloopDir],adapter.Manifest)`, `ContextState/SkillsState/AgentsState`, `RunSync(startDir,target)([]string,error)`, `profile.Profile{Tool}` are used identically across Tasks 1–4. The transient `Outputs`/`Render` (kept in Task 1, removed in Task 4) are the only intentionally short-lived symbols, and no v2 code path depends on them.
