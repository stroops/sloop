# Sloop `init --scan` — Heuristic Codebase Scan — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `sloop init --scan` — a pure-Go heuristic that reads an existing repo and writes a populated `AGENTS.md` (language, build/test commands, layout, README seed + a `## Conventions` fill-in) instead of the empty starter. No LLM, no new modules.

**Architecture:** New `internal/scan` package (mirrors `internal/detect` style: small structs, deterministic, best-effort, root-level reads only). `internal/sync` gains `EnsureAgentsContent(root, content)` so `init` can supply scanned content while keeping create-if-missing. `RunInit` gains a `scan bool`. Spec of record: `docs/superpowers/specs/2026-06-26-sloop-init-scan-design.md`.

## Global Constraints

- **Non-LLM, deterministic.** `Scan` never shells out, never walks the tree (root entries + a fixed set of manifest files only), never returns an error.
- **Create-if-missing always holds:** `--scan` never overwrites an existing `AGENTS.md`.
- Plain Markdown output — **no sloop markers** (Model B).
- No new Go modules; do **not** run `go mod tidy`. `go build ./...` + focused `go test`.
- Commit subjects: `type: Capitalized subject` (feat/fix/refactor/docs/chore), no trailing period, no `Co-Authored-By` trailer.
- Every task leaves `go build ./...` and `go test ./...` green.

---

### Task 1: `internal/scan` package (detection + render)

**Files:**
- Create: `internal/scan/scan.go`, `internal/scan/render.go`
- Test: `internal/scan/scan_test.go`, `internal/scan/render_test.go`

**Interfaces:**
```go
type Report struct {
	Name      string
	Languages []Lang
	Commands  []Command
	Layout    []string
	Summary   string
}
type Lang struct{ Name, Version string }
type Command struct{ Label, Cmd string } // build|test|lint|run

func Scan(root string) Report
func (r Report) AgentsMarkdown() string
```

- [ ] **Step 1: Write failing tests**

`internal/scan/scan_test.go`:
```go
package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasCmd(r Report, cmd string) bool {
	for _, c := range r.Commands {
		if c.Cmd == cmd {
			return true
		}
	}
	return false
}

func hasLang(r Report, name string) bool {
	for _, l := range r.Languages {
		if l.Name == name {
			return true
		}
	}
	return false
}

func TestScanGoRepo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module github.com/acme/widget\n\ngo 1.26\n")
	if err := os.MkdirAll(filepath.Join(dir, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := Scan(dir)
	if r.Name != "widget" {
		t.Fatalf("name = %q, want widget", r.Name)
	}
	if !hasLang(r, "Go") {
		t.Fatalf("languages = %+v, want Go", r.Languages)
	}
	if !hasCmd(r, "go test ./...") {
		t.Fatalf("commands = %+v, want go test ./...", r.Commands)
	}
	wantDir := map[string]bool{"cmd": false, "internal": false}
	for _, d := range r.Layout {
		if _, ok := wantDir[d]; ok {
			wantDir[d] = true
		}
	}
	for d, found := range wantDir {
		if !found {
			t.Fatalf("layout missing %q: %+v", d, r.Layout)
		}
	}
}

func TestScanNodeTSRepo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"name":"webapp","scripts":{"build":"tsc","test":"vitest"}}`)
	write(t, dir, "tsconfig.json", "{}")
	r := Scan(dir)
	if r.Name != "webapp" {
		t.Fatalf("name = %q", r.Name)
	}
	if !hasLang(r, "TypeScript") {
		t.Fatalf("languages = %+v, want TypeScript", r.Languages)
	}
	if !hasCmd(r, "npm run build") || !hasCmd(r, "npm test") {
		t.Fatalf("commands = %+v", r.Commands)
	}
}

func TestScanMakefileWins(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module x\n\ngo 1.26\n")
	write(t, dir, "Makefile", "test:\n\tgo test ./...\nbuild:\n\tgo build ./...\n")
	r := Scan(dir)
	if !hasCmd(r, "make test") {
		t.Fatalf("Makefile target should win: %+v", r.Commands)
	}
}

func TestScanUnknownRepo(t *testing.T) {
	dir := t.TempDir()
	r := Scan(dir)
	if r.Name != filepath.Base(dir) {
		t.Fatalf("name = %q, want basename", r.Name)
	}
	if len(r.Languages) != 0 {
		t.Fatalf("want no languages, got %+v", r.Languages)
	}
}
```

`internal/scan/render_test.go`:
```go
package scan

import (
	"strings"
	"testing"
)

func TestAgentsMarkdownPopulated(t *testing.T) {
	r := Report{
		Name:      "widget",
		Languages: []Lang{{Name: "Go", Version: "1.26"}},
		Commands:  []Command{{Label: "test", Cmd: "go test ./..."}},
		Layout:    []string{"cmd", "internal"},
		Summary:   "A widget service.",
	}
	md := r.AgentsMarkdown()
	for _, want := range []string{"# AGENTS.md", "widget", "Go 1.26", "go test ./...", "internal", "## Conventions"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestAgentsMarkdownEmptyStillValid(t *testing.T) {
	md := Report{Name: "x"}.AgentsMarkdown()
	if !strings.Contains(md, "# AGENTS.md") || !strings.Contains(md, "## Conventions") {
		t.Fatalf("empty report markdown invalid:\n%s", md)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/scan/ -v`
Expected: FAIL (package/functions undefined).

- [ ] **Step 3: Implement `scan.go`**

Read only root entries + the fixed manifest set. Key helpers:
- `projectName(root)`: try `go.mod` module (last path element), `package.json` `name`, `Cargo.toml`/`pyproject.toml` name; else `filepath.Base(root)`.
- `languages(root)`: marker-file map → `[]Lang` (TypeScript when `tsconfig.json`, else JavaScript for `package.json`); attach version from `go.mod`/`engines`/`.python-version`.
- `commands(root, langs)`: start from per-language defaults (Go: `go build ./...`,`go test ./...`,`go vet ./...`; Rust: `cargo build`/`cargo test`/`cargo clippy`; Node: from `package.json` `scripts`, prefixed by the lockfile's package manager); then **override** any matching label with `make <target>` when a `Makefile` declares that target. Fixed label order: build, test, lint, run.
- `layout(root)`: one-level dir listing intersected with a known-meaningful set (`cmd internal pkg src app lib test tests docs api web server client services`), sorted.
- `readmeSeed(root)`: first `# H1` + first non-empty paragraph of `README.md`.

`Scan` assembles these into a `Report`. No error return; all reads best-effort. JSON parse of `package.json` via `encoding/json` (already in stdlib); `go.mod`/`Makefile` parsed with light string scanning (no new deps; do not import `golang.org/x/mod`).

- [ ] **Step 4: Implement `render.go`**

`AgentsMarkdown()` builds the scaffold per spec §5: H1 + intro line; `## Project` (name + `<!-- one-line -->` + quoted Summary if present); `## Tech stack` (each `Lang` as `- <Name> [<Version>] _(detected)_`, omitted if none); `## Build, test & lint` fenced `sh` block of the commands (omitted if none); `## Project structure` (each layout dir as a bullet, omitted if none); always-present `## Conventions` with the fill-in comment. Use a `strings.Builder`.

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/scan/ -v && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/scan/
git commit -m "feat: Add internal/scan heuristic codebase scanner"
```

---

### Task 2: Wire `sloop init --scan`

**Files:**
- Modify: `internal/sync/deliver.go` (add `EnsureAgentsContent`; `EnsureAgents` delegates)
- Modify: `internal/cli/commands/init.go` (`RunInit(dir, scan bool)`; `--scan` flag)
- Modify callers passing `RunInit(dir)` → `RunInit(dir, false)`: `internal/cli/commands/{initcmd_test,runcmd_test,runwflag_test,synccmd_test}.go` (whichever call it)
- Test: `internal/sync/deliver_test.go`, `internal/cli/commands/initcmd_test.go`

**Interfaces:**
- `func EnsureAgentsContent(root, content string) (Action, error)`
- `func RunInit(dir string, scan bool) error`

- [ ] **Step 1: Write failing tests**

In `internal/sync/deliver_test.go`:
```go
func TestEnsureAgentsContentCreatesThenSkips(t *testing.T) {
	root := t.TempDir()
	a, err := EnsureAgentsContent(root, "# AGENTS.md\nscanned\n")
	if err != nil || a != ActionCreated {
		t.Fatalf("create = %v, %v", a, err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if string(b) != "# AGENTS.md\nscanned\n" {
		t.Fatalf("content = %q", string(b))
	}
	a, _ = EnsureAgentsContent(root, "DIFFERENT")
	if a != ActionSkipped {
		t.Fatalf("second = %v, want skipped", a)
	}
}
```

In `internal/cli/commands/initcmd_test.go` add:
```go
func TestRunInitScanPopulatesAgents(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunInit(dir, true); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "go test ./...") {
		t.Fatalf("scanned AGENTS.md should contain build commands:\n%s", string(b))
	}
}
```
(Ensure the existing `TestRunInitScaffolds` updates its `RunInit(dir)` call to `RunInit(dir, false)` and keeps its AGENTS.md-present assertion.)

- [ ] **Step 2: Run tests to verify they fail / build breaks**

Run: `go test ./internal/sync/ ./internal/cli/commands/ -run 'EnsureAgentsContent|RunInitScan' 2>&1 | head`
Expected: FAIL/compile error (undefined `EnsureAgentsContent`; `RunInit` arity).

- [ ] **Step 3: Add `EnsureAgentsContent` and delegate**

In `internal/sync/deliver.go`:
```go
func EnsureAgentsContent(root, content string) (Action, error) {
	path := filepath.Join(root, "AGENTS.md")
	if _, err := os.Stat(path); err == nil {
		return ActionSkipped, nil
	} else if !os.IsNotExist(err) {
		return ActionSkipped, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return ActionSkipped, err
	}
	return ActionCreated, nil
}

func EnsureAgents(root string) (Action, error) {
	return EnsureAgentsContent(root, agentsStarter)
}
```

- [ ] **Step 4: Wire `RunInit(dir, scan)` + `--scan` flag**

In `init.go`, change `RunInit(dir string)` → `RunInit(dir string, scan bool)`. Replace the
`syncpkg.EnsureAgents(dir)` call with:
```go
	if scan {
		if _, err := syncpkg.EnsureAgentsContent(dir, scan.Scan(dir).AgentsMarkdown()); err != nil {
			return err
		}
	} else {
		if _, err := syncpkg.EnsureAgents(dir); err != nil {
			return err
		}
	}
```
(Import `"github.com/stroops/sloop/internal/scan"`; note the local var name clash — call the package `scanpkg` if needed, since `scan` is also the bool param: `scanpkg.Scan(dir)`.) Add the flag in `RegisterInit`/`initCmd`:
```go
var initScan bool
// in initCmd RunE: pass initScan
// in RegisterInit: initCmd.Flags().BoolVar(&initScan, "scan", false, "scan the existing codebase to pre-fill AGENTS.md")
```
Update the `initCmd` `RunE` to call `RunInit(dir, initScan)`.

- [ ] **Step 5: Update other `RunInit` callers**

Change every `RunInit(dir)` to `RunInit(dir, false)` in the test files that call it
(`runcmd_test.go`, `runwflag_test.go`, `synccmd_test.go`, and any in `initcmd_test.go`). Run
`go build ./... && go vet ./...` to surface any missed caller.

- [ ] **Step 6: Run full suite + build**

Run: `go build ./... && go test ./...`
Expected: all PASS.

- [ ] **Step 7: End-to-end smoke**

```bash
go build -o /tmp/sloop ./cmd/sloop
export HOME="$(mktemp -d)"
WORK="$(mktemp -d)/demo"; mkdir -p "$WORK"; cd "$WORK"
printf 'module demo\n\ngo 1.26\n' > go.mod; mkdir -p cmd internal
/tmp/sloop init --scan
echo "--- AGENTS.md ---"; cat AGENTS.md
```
Expected: `AGENTS.md` lists Go 1.26, `go test ./...`, `cmd`/`internal` layout, and a `## Conventions` placeholder — not the empty starter.

- [ ] **Step 8: Commit**

```bash
git add internal/sync/ internal/cli/commands/
git commit -m "feat: Add sloop init --scan to pre-fill AGENTS.md from the codebase"
```

---

## Self-Review

**Spec coverage:**
- §4 detection signals (name, languages+version, commands, layout, README seed) → Task 1 ✓
- §4 Makefile precedence + Node scripts → Task 1 (`commands`) ✓
- §5 scaffold output + always-present `## Conventions` + omit-empty sections → Task 1 (`render.go`) ✓
- §6 `--scan` flag, create-if-missing, `RunInit(dir, scan)` → Task 2 ✓
- §6 `EnsureAgentsContent` + `EnsureAgents` delegation → Task 2 ✓
- §8 error handling (Scan never errors; degraded report) → Task 1 (`TestScanUnknownRepo`) ✓
- §9 testing (Go/Node/Makefile/empty/README/integration) → both tasks' TDD + e2e ✓

**Non-regression:** `EnsureAgents(root)` keeps its signature and exact starter output (now via
delegation), so all existing v2 delivery/state tests stay green. `--scan` is opt-in; default
`init` is byte-identical to today. `Scan` adds no module and no network/exec.

**Type consistency:** `scan.Report{Name,Languages,Commands,Layout,Summary}`, `Scan(root) Report`,
`Report.AgentsMarkdown() string`, `sync.EnsureAgentsContent(root,content)`, `RunInit(dir,scan)` are
used identically across Tasks 1–2.
