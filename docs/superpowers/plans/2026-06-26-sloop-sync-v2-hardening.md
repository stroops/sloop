# Sloop Sync v2 — Hardening / Gap-Fill — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the three remaining gaps between the shipped Sync v2 (Model B) and its design, without re-introducing the obsolete two-way engine: (1) `sloop sync --all`, (2) robust **relative** skills symlinks with a real `broken` state and self-heal, (3) opt-in non-destructive `sloop sync --repair`.

**Architecture:** All work stays in the `internal/sync` delivery layer plus the `sync` command. Skills symlinks become relative (`../.sloop/skills`) via a shared `skillsPaths` helper used by both delivery and state, so they cannot disagree. A `syncOne` helper is extracted so single-tool and `--all` paths share one delivery block. `RepairContext`/`RepairSkills` are added alongside (not inside) the create-if-missing functions, so existing callers are untouched. No adapter/config/manifest changes; no new Go modules.

**Tech Stack:** Go 1.26, Cobra, `os` symlink/file APIs. Spec of record: `docs/superpowers/specs/2026-06-26-sloop-sync-v2-hardening-design.md`.

## Global Constraints

- Builds on Model B (`2026-06-26-sloop-sync-v2-design.md`). Delivery philosophy unchanged: **create-if-missing, never overwrite, no markers**. `--repair` is the only new mutation and it is **non-destructive** (rename aside to `*.sloopbak-<ts>`, never delete; never touches `AGENTS.md`).
- Skills symlinks are **relative**; a single `skillsPaths(root, sloopDir, target)` helper is the only place that computes link/source/rel.
- `--all` reads `config.Project.Tools`; `--all` + positional arg = usage error; one bad tool warns and is skipped (don't abort the batch).
- No new Go modules; do **not** run `go mod tidy`. Use `go build ./...` + focused `go test`.
- Commit subjects: `type: Capitalized subject` (type ∈ feat/fix/refactor/docs/chore), no trailing period, no `Co-Authored-By` trailer.
- Every task must leave `go build ./...` and `go test ./...` green.

---

### Task 1: Relative skills symlink + `broken` state + self-heal

**Files:**
- Modify: `internal/sync/deliver.go` (add `skillsPaths`/`isOurLink` helpers; relative symlink; `ActionBroken`/`ActionRelinked`; self-heal in `SyncSkills`)
- Modify: `internal/sync/state.go` (`SkillsState` uses the shared helper; adds `broken`)
- Test: extend `internal/sync/deliver_test.go`, `internal/sync/state_test.go`

**Interfaces:**
- Produces:
  - `func skillsPaths(root, sloopDir, manifestTarget string) (link, source, rel string)`
  - `func isOurLink(readlinkDst, source, rel string) bool`
  - `ActionBroken Action = "broken"`, `ActionRelinked Action = "relinked"`
  - `SkillsState(...)` now returns `none|missing|linked|broken|present`

- [ ] **Step 1: Write/extend failing tests**

In `internal/sync/deliver_test.go` add:
```go
func TestSyncSkillsRelativeLink(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}
	if a, err := SyncSkills(root, sloopDir, m); err != nil || a != ActionLinked {
		t.Fatalf("link = %v, %v", a, err)
	}
	dst, err := os.Readlink(filepath.Join(root, ".claude", "skills"))
	if err != nil || dst != filepath.Join("..", ".sloop", "skills") {
		t.Fatalf("want relative ../.sloop/skills, got %q (%v)", dst, err)
	}
}

func TestSyncSkillsHealsLegacyAbsoluteLink(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-hardening absolute symlink.
	if err := os.Symlink(filepath.Join(sloopDir, "skills"), link); err != nil {
		t.Fatal(err)
	}
	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}
	if a, err := SyncSkills(root, sloopDir, m); err != nil || a != ActionRelinked {
		t.Fatalf("relink = %v, %v", a, err)
	}
	dst, _ := os.Readlink(link)
	if dst != filepath.Join("..", ".sloop", "skills") {
		t.Fatalf("want relative after heal, got %q", dst)
	}
}
```

In `internal/sync/state_test.go` add:
```go
func TestSkillsStateBrokenAndPresent(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}
	if _, err := SyncSkills(root, sloopDir, m); err != nil {
		t.Fatal(err)
	}
	// Remove the source → our link is now broken.
	if err := os.RemoveAll(filepath.Join(sloopDir, "skills")); err != nil {
		t.Fatal(err)
	}
	if got := SkillsState(root, sloopDir, m); got != "broken" {
		t.Fatalf("want broken, got %s", got)
	}
	// A real foreign dir reads as present.
	root2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root2, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := SkillsState(root2, filepath.Join(root2, ".sloop"), m); got != "present" {
		t.Fatalf("want present, got %s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sync/ -run 'Relative|Legacy|BrokenAndPresent' -v`
Expected: FAIL (undefined `ActionRelinked`/`ActionBroken`; absolute link still produced; `broken` not detected).

- [ ] **Step 3: Implement the helpers + relative link + self-heal**

In `deliver.go`, add the action constants and helpers, and rewrite `SyncSkills`:
```go
const (
	ActionBroken   Action = "broken"
	ActionRelinked Action = "relinked"
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

	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
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
```
Add the `config` import (`"github.com/stroops/sloop/internal/config"` — no cycle; `config` imports only yaml). Note `symlinkFunc(rel, link)` now passes the **relative** source.

In `state.go`, rewrite `SkillsState` to use the shared helper:
```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sync/ -v && go build ./...`
Expected: PASS — new tests green; existing `TestSyncSkillsSymlinkThenIdempotent`, `TestSyncSkillsCopyFallback`, `TestSkillsStateLinked` still green (they don't assert the absolute target).

- [ ] **Step 5: Commit**

```bash
git add internal/sync/deliver.go internal/sync/state.go internal/sync/deliver_test.go internal/sync/state_test.go
git commit -m "feat: Make skills symlinks relative with broken-state and self-heal"
```

---

### Task 2: `sloop sync --all`

**Files:**
- Modify: `internal/cli/commands/sync.go` (extract `syncOne`; add `RunSyncAll`; add `--all` flag)
- Test: `internal/cli/commands/synccmd_test.go`

**Interfaces:**
- Produces:
  - `func syncOne(root, sloopDir string, m adapter.Manifest) ([]string, error)` (shared delivery block)
  - `func RunSyncAll(startDir string) ([]string, error)` (iterates `proj.Tools`, prefixes lines with tool key)

- [ ] **Step 1: Write the failing test**

In `synccmd_test.go` add (assumes the existing test helper that scaffolds a workspace with `init`; enable a second tool in project config):
```go
func TestRunSyncAllDeliversEnabledTools(t *testing.T) {
	dir := scaffoldWorkspace(t) // existing helper used by TestRunSyncWritesClaudeMd
	// Enable claude + cursor.
	if err := config.SaveProject(filepath.Join(dir, ".sloop"), &config.Project{
		Tools: []string{"claude", "cursor"}, DefaultTool: "claude",
	}); err != nil {
		t.Fatal(err)
	}
	lines, err := RunSyncAll(dir)
	if err != nil {
		t.Fatalf("RunSyncAll: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatalf("claude pointer missing: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "claude:") {
		t.Fatalf("expected per-tool prefixed output, got:\n%s", joined)
	}
}
```
(If `scaffoldWorkspace` doesn't exist by that name, reuse whatever setup `TestRunSyncWritesClaudeMd` already does.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands/ -run TestRunSyncAll -v`
Expected: FAIL (undefined `RunSyncAll`).

- [ ] **Step 3: Extract `syncOne` and add `RunSyncAll`**

In `sync.go`, factor the delivery block currently inside `RunSync` into `syncOne`:
```go
func syncOne(root, sloopDir string, m adapter.Manifest) ([]string, error) {
	var log []string
	if a, err := syncpkg.EnsureAgents(root); err != nil {
		return nil, err
	} else if a == syncpkg.ActionCreated {
		log = append(log, "created AGENTS.md")
	}
	switch a, err := syncpkg.SyncContext(root, m); {
	case err != nil:
		return nil, err
	case a == syncpkg.ActionCreated:
		log = append(log, "created "+m.Context.File)
	case a == syncpkg.ActionForeign:
		log = append(log, m.Context.File+" exists, left as-is")
	}
	switch a, err := syncpkg.SyncSkills(root, sloopDir, m); {
	case err != nil:
		return nil, err
	case a == syncpkg.ActionLinked:
		log = append(log, "linked "+m.Skills.Target)
	case a == syncpkg.ActionRelinked:
		log = append(log, "relinked "+m.Skills.Target)
	case a == syncpkg.ActionCopied:
		log = append(log, "copied skills to "+m.Skills.Target)
	case a == syncpkg.ActionBroken:
		log = append(log, "skills source .sloop/skills missing (left "+m.Skills.Target+")")
	case a == syncpkg.ActionForeign:
		log = append(log, m.Skills.Target+" exists, left as-is")
	}
	return log, nil
}
```
Have `RunSync` call `syncOne(ws.Root, ws.SloopDir(), m)` after resolving the profile/manifest (keep its existing resolve logic). Add `RunSyncAll`:
```go
func RunSyncAll(startDir string) ([]string, error) {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return nil, err
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return nil, err
	}
	manifests, err := adapter.Load()
	if err != nil {
		return nil, err
	}
	var log []string
	for _, tool := range proj.Tools {
		m, ok := manifests[tool]
		if !ok {
			log = append(log, tool+": unknown tool (no adapter), skipped")
			continue
		}
		lines, err := syncOne(ws.Root, ws.SloopDir(), m)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", tool, err)
		}
		for _, l := range lines {
			log = append(log, tool+": "+l)
		}
	}
	return log, nil
}
```

- [ ] **Step 4: Wire the `--all` flag**

In the `syncCmd` `RunE`, branch on a package-level `syncAll bool`:
```go
if syncAll {
	if len(args) > 0 {
		return fmt.Errorf("--all takes no tool argument")
	}
	written, err := RunSyncAll(startDir)
	// ... print loop, return
}
```
Register it in `RegisterSync`:
```go
syncCmd.Flags().BoolVar(&syncAll, "all", false, "sync every enabled tool")
```

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/cli/commands/ -run 'TestRunSync' -v && go build ./...`
Expected: PASS (new `--all` test + existing single-tool sync test).

- [ ] **Step 6: Commit**

```bash
git add internal/cli/commands/sync.go internal/cli/commands/synccmd_test.go
git commit -m "feat: Add sloop sync --all for every enabled tool"
```

---

### Task 3: `sloop sync --repair` (non-destructive backup)

**Files:**
- Modify: `internal/sync/deliver.go` (`ActionRepaired`; `RepairContext`, `RepairSkills`, `backupAside`)
- Modify: `internal/cli/commands/sync.go` (`--repair` flag; repair-aware delivery in `syncOne`)
- Test: `internal/sync/deliver_test.go`

**Interfaces:**
- Produces:
  - `ActionRepaired Action = "repaired"`
  - `func RepairContext(root string, m adapter.Manifest) (Action, error)`
  - `func RepairSkills(root, sloopDir string, m adapter.Manifest) (Action, error)`
  - `syncOne(root, sloopDir string, m adapter.Manifest, repair bool) ([]string, error)` (add the bool)

- [ ] **Step 1: Write the failing tests**

```go
func TestRepairContextBacksUpForeign(t *testing.T) {
	root := t.TempDir()
	m := adapter.Manifest{Name: "Claude Code", Context: adapter.ContextSpec{Mode: "pointer", File: "CLAUDE.md"}}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("MINE"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := RepairContext(root, m)
	if err != nil || a != ActionRepaired {
		t.Fatalf("repair = %v, %v", a, err)
	}
	// Pointer now written.
	b, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if !strings.Contains(string(b), "AGENTS.md") {
		t.Fatalf("pointer not written: %q", string(b))
	}
	// Original preserved under a *.sloopbak-* name.
	matches, _ := filepath.Glob(filepath.Join(root, "CLAUDE.md.sloopbak-*"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 backup, got %v", matches)
	}
	if bk, _ := os.ReadFile(matches[0]); string(bk) != "MINE" {
		t.Fatalf("backup content lost: %q", string(bk))
	}
}

func TestRepairSkillsBacksUpPresentDir(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	if err := os.MkdirAll(filepath.Join(sloopDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A foreign real dir occupies the target.
	if err := os.MkdirAll(filepath.Join(root, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude", "skills", "keep.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := adapter.Manifest{Skills: adapter.SkillsSpec{Target: ".claude/skills"}}
	a, err := RepairSkills(root, sloopDir, m)
	if err != nil || a != ActionRepaired {
		t.Fatalf("repair = %v, %v", a, err)
	}
	fi, err := os.Lstat(filepath.Join(root, ".claude", "skills"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target should now be a symlink: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(root, ".claude", "skills.sloopbak-*"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 skills backup, got %v", matches)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sync/ -run TestRepair -v`
Expected: FAIL (undefined `RepairContext`/`RepairSkills`/`ActionRepaired`).

- [ ] **Step 3: Implement repair (non-destructive)**

In `deliver.go`:
```go
const ActionRepaired Action = "repaired"

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
```
Add the `time` import. `RepairContext` must never touch `AGENTS.md` — it only acts on `m.Context.File`, so that invariant holds by construction.

- [ ] **Step 4: Thread `repair` through the command**

In `sync.go`, change `syncOne` to take `repair bool` and pick the repair variants for the two channels when set:
```go
func syncOne(root, sloopDir string, m adapter.Manifest, repair bool) ([]string, error) {
	// AGENTS.md ensure (unchanged) ...
	ctx := syncpkg.SyncContext
	skl := syncpkg.SyncSkills
	if repair {
		ctx = syncpkg.RepairContext
		skl = func(r, s string, mm adapter.Manifest) (syncpkg.Action, error) { return syncpkg.RepairSkills(r, s, mm) }
	}
	// ... use ctx(root, m) and skl(root, sloopDir, m); add ActionRepaired cases to the log switches
}
```
(`SyncContext` and `RepairContext` share the `func(string, adapter.Manifest) (Action, error)` signature; assign directly.) Add a package-level `syncRepair bool`, register `--repair`, and pass it from both `RunSync` and `RunSyncAll` (add the param to both; `--repair` composes with `--all`). Add an `ActionRepaired` case to the context and skills log switches (`repaired <file>` / `repaired <target>`).

- [ ] **Step 5: Run the suite + build**

Run: `go test ./... && go build ./...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/sync/deliver.go internal/sync/deliver_test.go internal/cli/commands/sync.go
git commit -m "feat: Add sloop sync --repair with non-destructive backup"
```

---

### Task 4: End-to-end smoke + docs

**Files:**
- Verify: backlog Phase 3.2 annotated superseded (done in design step; confirm present)
- No code; integration validation only.

- [ ] **Step 1: Build and smoke `--all`, move-survival, `--repair`**

```bash
go build -o /tmp/sloop ./cmd/sloop
export HOME="$(mktemp -d)"
BASE="$(mktemp -d)"; WORK="$BASE/svc"; mkdir -p "$WORK"; cd "$WORK"
/tmp/sloop init
# enable a second tool by editing .sloop/config.yaml tools: [claude, cursor]
/tmp/sloop sync --all
test -L .claude/skills && readlink .claude/skills   # expect ../.sloop/skills
/tmp/sloop status                                    # skills:linked
mv "$WORK" "$BASE/svc-moved"; cd "$BASE/svc-moved"
/tmp/sloop status                                    # still skills:linked (relative link survived)
printf 'MINE\n' > CLAUDE.md                          # make it foreign
/tmp/sloop sync                                      # warns: CLAUDE.md exists, left as-is
/tmp/sloop sync --repair                             # backs up + writes pointer
ls CLAUDE.md.sloopbak-*                              # backup exists
```
Expected: relative symlink; status survives the move; `--repair` backs up the foreign `CLAUDE.md` and writes the pointer; `AGENTS.md` untouched throughout.

- [ ] **Step 2: Full suite**

Run: `go build ./... && go test ./...`
Expected: all PASS.

- [ ] **Step 3: Confirm backlog/spec docs**

Verify `docs/plans/sloop-backlog.md` Phase 3.2 is annotated **superseded** (pointing at the hardening spec) and `docs/superpowers/specs/2026-06-26-sloop-sync-v2-hardening-design.md` exists. Commit any doc tidy-ups:
```bash
git add docs/
git commit -m "docs: Mark backlog Phase 3.2 superseded by sync v2 hardening"
```

---

## Self-Review

**Spec coverage:**
- §3 `sync --all` (enabled tools, prefixed output, bad-tool-skipped, usage error) → Task 2 ✓
- §4 relative symlink + `broken` + self-heal + reconciled `SkillsState` → Task 1 ✓
- §5 `--repair` non-destructive backup (context + skills; never AGENTS.md; composes with `--all`) → Task 3 ✓
- §8 error handling (usage error, skip bad tool, repair-rename-fail aborts that artifact, broken source reported) → Tasks 2,3 ✓
- §10 backlog reconciliation → design step + Task 4 ✓
- §11 testing (relative, move survival, broken, repair-preserves-bytes, status vocab, integration) → each task's TDD + Task 4 smoke ✓

**Non-destructive guarantee:** `--repair` only ever `os.Rename`s the occupant to `*.sloopbak-<ts>` before creating sloop's artifact; no `os.Remove`/`RemoveAll` of user content anywhere. `AGENTS.md` is never a repair target (`RepairContext` acts only on `m.Context.File`).

**No model regression:** default `sync` (no flags) keeps the exact Model B create-if-missing/never-overwrite behavior; relative-vs-absolute symlink is the only behavioral change to the default path, and it is a strict robustness improvement (existing tests assert symlink presence, not target, so they stay green).

**Type consistency:** `skillsPaths`/`isOurLink` are the single source of link/source/rel and are shared by `SyncSkills`, `SkillsState`, and `RepairSkills`. `Action` gains `ActionBroken`/`ActionRelinked`/`ActionRepaired`; `syncOne(root,sloopDir,m,repair)` is the one delivery block used by `RunSync` and `RunSyncAll`.
