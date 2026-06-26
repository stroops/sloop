# Sloop Core Vertical Slice — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `sloop init` then `sloop run claude` work end-to-end — scaffold a `.sloop/` workspace, sync canonical context into `CLAUDE.md`, register the workspace + session in SQLite, and launch the tool in the workspace root.

**Architecture:** A single `sloop` binary (Cobra). Canonical context lives in `.sloop/`; an `adapter` renders it into a tool's native file; `runner` launches the tool with `cwd` = workspace root. State (workspaces registry + session history) is a local SQLite DB at `~/.sloop/sloop.db`. No daemon. tmux/detection are deferred to Plan 2.

**Tech Stack:** Go 1.26, Cobra, `modernc.org/sqlite` (pure-Go, no cgo), `gopkg.in/yaml.v3`, `go:embed` for built-in adapter manifests.

## Global Constraints

- Go version floor: `go 1.26` (module already declares `go 1.26.4`).
- SQLite driver: `modernc.org/sqlite` only (pure-Go, cross-platform — no cgo).
- YAML library: `gopkg.in/yaml.v3` only (already in the module graph).
- Single binary `sloop`; **no daemon, no gRPC, no network**. Every command runs to completion.
- Global state path: `~/.sloop/` (DB, user adapters). Project state path: `<root>/.sloop/`.
- Generated native files (e.g. `CLAUDE.md`) are written at the workspace **root**, not inside `.sloop/`.
- Commit subjects must be `type: Capitalized subject` (type ∈ feat/fix/refactor/docs/chore/perf/revert), no trailing period, no `Co-Authored-By` trailer.
- Spec of record: `docs/superpowers/specs/2026-06-26-sloop-mvp-design.md`.

---

### Task 1: Remove daemon scaffolding and add dependencies

**Files:**
- Delete: `cmd/sloopd/` (entire directory)
- Delete: `internal/store/sqlite.go` (broken; replaced by `internal/session` in Task 3)
- Modify: `internal/cli/root.go` (remove the `--socket` flag and its viper binding)
- Modify: `go.mod` / `go.sum` (add `modernc.org/sqlite`, `gopkg.in/yaml.v3`)

**Interfaces:**
- Consumes: nothing.
- Produces: a clean-building module with the two new direct dependencies available.

- [ ] **Step 1: Delete the daemon and broken store**

```bash
git rm -r cmd/sloopd
git rm internal/store/sqlite.go
```

- [ ] **Step 2: Remove the `--socket` flag from `internal/cli/root.go`**

In `func init()`, delete these three lines:

```go
	rootCmd.PersistentFlags().String("socket", filepath.Join(os.TempDir(), "sloop.sock"), "daemon socket path")

	viper.BindPFlag("socket", rootCmd.PersistentFlags().Lookup("socket"))
```

Keep the `--config` and `--no-color` flags. If `filepath` becomes unused after this edit, remove it from the import block (it is still used by `initConfig`, so it stays).

- [ ] **Step 3: Add dependencies**

Run:
```bash
go get modernc.org/sqlite@latest
go get gopkg.in/yaml.v3@v3.0.1
go mod tidy
```

- [ ] **Step 4: Verify the module builds and the daemon is gone**

Run: `go build ./... && test ! -d cmd/sloopd && echo OK`
Expected: prints `OK` with no build errors.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "chore: Remove daemon scaffolding and add sqlite/yaml deps"
```

---

### Task 2: `internal/config` — paths and config types

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `func GlobalDir() (string, error)` → `~/.sloop`
  - `func GlobalDBPath() (string, error)` → `~/.sloop/sloop.db`
  - `func UserAdaptersDir() (string, error)` → `~/.sloop/adapters`
  - `const SloopDirName = ".sloop"`
  - `type Project struct { Tools []string; DefaultTool string }` (yaml: `tools`, `default_tool`)
  - `func LoadProject(sloopDir string) (*Project, error)` (reads `<sloopDir>/config.yaml`)
  - `func SaveProject(sloopDir string, p *Project) error`

- [ ] **Step 1: Write the failing test**

```go
package config

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoadProject(t *testing.T) {
	dir := t.TempDir()
	p := &Project{Tools: []string{"claude"}, DefaultTool: "claude"}
	if err := SaveProject(dir, p); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	if _, err := filepath.Glob(filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatalf("glob: %v", err)
	}
	got, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if got.DefaultTool != "claude" || len(got.Tools) != 1 || got.Tools[0] != "claude" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestGlobalDBPath(t *testing.T) {
	p, err := GlobalDBPath()
	if err != nil {
		t.Fatalf("GlobalDBPath: %v", err)
	}
	if filepath.Base(p) != "sloop.db" {
		t.Fatalf("want sloop.db, got %s", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestSaveAndLoadProject -v`
Expected: FAIL (undefined: SaveProject / Project / LoadProject).

- [ ] **Step 3: Write minimal implementation**

```go
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const SloopDirName = ".sloop"

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

// Project is the per-project config stored at <sloopDir>/config.yaml.
type Project struct {
	Tools       []string `yaml:"tools"`
	DefaultTool string   `yaml:"default_tool"`
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
	if err := os.MkdirAll(sloopDir, 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sloopDir, "config.yaml"), b, 0o644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: Add config package with paths and project config"
```

---

### Task 3: `internal/session` — SQLite store (workspaces + sessions)

**Files:**
- Create: `internal/session/store.go`
- Test: `internal/session/store_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `func Open(path string) (*Store, error)` (creates parent dir, runs migrations)
  - `func (*Store) Close() error`
  - `type Workspace struct { ID int64; Name, Path string; CreatedAt time.Time }`
  - `func (*Store) RegisterWorkspace(name, path string) (*Workspace, error)` (idempotent upsert keyed by `path`)
  - `func (*Store) WorkspaceByName(name string) (*Workspace, error)`
  - `func (*Store) ListWorkspaces() ([]Workspace, error)`
  - `type Session struct { ID, WorkspaceID int64; Tool, Profile, Cwd, TmuxSession string; StartedAt time.Time; EndedAt *time.Time }`
  - `func (*Store) RecordSession(s Session) (int64, error)`
  - `func (*Store) EndSession(id int64, ended time.Time) error`
  - `func (*Store) ListSessions(limit int) ([]Session, error)`

- [ ] **Step 1: Write the failing test**

```go
package session

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "sloop.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRegisterWorkspaceIdempotent(t *testing.T) {
	s := newTestStore(t)
	w1, err := s.RegisterWorkspace("backend", "/tmp/backend")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	w2, err := s.RegisterWorkspace("backend", "/tmp/backend")
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if w1.ID != w2.ID {
		t.Fatalf("expected same id, got %d and %d", w1.ID, w2.ID)
	}
	got, err := s.WorkspaceByName("backend")
	if err != nil {
		t.Fatalf("by name: %v", err)
	}
	if got.Path != "/tmp/backend" {
		t.Fatalf("want /tmp/backend, got %s", got.Path)
	}
}

func TestRecordAndListSessions(t *testing.T) {
	s := newTestStore(t)
	w, _ := s.RegisterWorkspace("backend", "/tmp/backend")
	id, err := s.RecordSession(Session{
		WorkspaceID: w.ID, Tool: "claude", Cwd: "/tmp/backend", StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := s.EndSession(id, time.Now()); err != nil {
		t.Fatalf("end: %v", err)
	}
	list, err := s.ListSessions(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Tool != "claude" || list[0].EndedAt == nil {
		t.Fatalf("unexpected sessions: %+v", list)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/session/ -v`
Expected: FAIL (undefined: Open / Store / RegisterWorkspace / …).

- [ ] **Step 3: Write minimal implementation**

```go
package session

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS workspaces (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT UNIQUE NOT NULL,
  path       TEXT UNIQUE NOT NULL,
  created_at TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL REFERENCES workspaces(id),
  tool         TEXT NOT NULL,
  profile      TEXT,
  cwd          TEXT,
  tmux_session TEXT,
  started_at   TIMESTAMP NOT NULL,
  ended_at     TIMESTAMP
);`

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

type Workspace struct {
	ID        int64
	Name      string
	Path      string
	CreatedAt time.Time
}

func (s *Store) RegisterWorkspace(name, path string) (*Workspace, error) {
	_, err := s.db.Exec(
		`INSERT INTO workspaces(name, path, created_at) VALUES(?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET name=excluded.name`,
		name, path, time.Now(),
	)
	if err != nil {
		return nil, err
	}
	return s.workspaceByPath(path)
}

func (s *Store) workspaceByPath(path string) (*Workspace, error) {
	var w Workspace
	err := s.db.QueryRow(
		`SELECT id, name, path, created_at FROM workspaces WHERE path = ?`, path,
	).Scan(&w.ID, &w.Name, &w.Path, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (s *Store) WorkspaceByName(name string) (*Workspace, error) {
	var w Workspace
	err := s.db.QueryRow(
		`SELECT id, name, path, created_at FROM workspaces WHERE name = ?`, name,
	).Scan(&w.ID, &w.Name, &w.Path, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (s *Store) ListWorkspaces() ([]Workspace, error) {
	rows, err := s.db.Query(`SELECT id, name, path, created_at FROM workspaces ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Workspace
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Path, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

type Session struct {
	ID          int64
	WorkspaceID int64
	Tool        string
	Profile     string
	Cwd         string
	TmuxSession string
	StartedAt   time.Time
	EndedAt     *time.Time
}

func (s *Store) RecordSession(sess Session) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO sessions(workspace_id, tool, profile, cwd, tmux_session, started_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		sess.WorkspaceID, sess.Tool, sess.Profile, sess.Cwd, sess.TmuxSession, sess.StartedAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) EndSession(id int64, ended time.Time) error {
	_, err := s.db.Exec(`UPDATE sessions SET ended_at = ? WHERE id = ?`, ended, id)
	return err
}

func (s *Store) ListSessions(limit int) ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, workspace_id, tool, COALESCE(profile,''), COALESCE(cwd,''),
		        COALESCE(tmux_session,''), started_at, ended_at
		 FROM sessions ORDER BY started_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.Tool, &s.Profile, &s.Cwd,
			&s.TmuxSession, &s.StartedAt, &s.EndedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/session/
git commit -m "feat: Add SQLite session store for workspaces and history"
```

---

### Task 4: `internal/workspace` — directory resolution

**Files:**
- Create: `internal/workspace/workspace.go`
- Test: `internal/workspace/workspace_test.go`

**Interfaces:**
- Consumes: `config.SloopDirName`.
- Produces:
  - `type Workspace struct { Name, Root string }`
  - `func (w Workspace) SloopDir() string` → `<Root>/.sloop`
  - `func Resolve(startDir string) (*Workspace, error)` — walk up from `startDir` to the nearest ancestor containing `.sloop/`; `Name` defaults to `filepath.Base(Root)`. Returns `ErrNotFound` if none.
  - `var ErrNotFound = errors.New("no .sloop workspace found")`

- [ ] **Step 1: Write the failing test**

```go
package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".sloop"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := Resolve(nested)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	gotRoot, _ := filepath.EvalSymlinks(w.Root)
	wantRoot, _ := filepath.EvalSymlinks(root)
	if gotRoot != wantRoot {
		t.Fatalf("want root %s, got %s", wantRoot, gotRoot)
	}
	if w.Name != filepath.Base(wantRoot) {
		t.Fatalf("want name %s, got %s", filepath.Base(wantRoot), w.Name)
	}
}

func TestResolveNotFound(t *testing.T) {
	_, err := Resolve(t.TempDir())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/workspace/ -v`
Expected: FAIL (undefined: Resolve / Workspace / ErrNotFound).

- [ ] **Step 3: Write minimal implementation**

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/workspace/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/workspace/
git commit -m "feat: Add workspace resolution by walking up to .sloop"
```

---

### Task 5: `internal/adapter` — declarative YAML manifests

**Files:**
- Create: `internal/adapter/adapter.go`
- Create: `internal/adapter/builtin/claude.yaml`
- Test: `internal/adapter/adapter_test.go`

**Interfaces:**
- Consumes: nothing (Plan 1 loads built-ins only; user adapters added in Plan 2).
- Produces:
  - `type Output struct { Path, Template string }`
  - `type Manifest struct { Name, Detect, Launch string; Outputs []Output }`
  - `func LoadBuiltin() (map[string]Manifest, error)` — keyed by filename without `.yaml` (e.g. `claude`)
  - `func (m Manifest) Render(assembled string) map[string]string` — maps each output `Path` → content; template `"default"` yields `assembled` verbatim.

- [ ] **Step 1: Create the built-in manifest**

`internal/adapter/builtin/claude.yaml`:
```yaml
name: Claude Code
detect: claude
launch: claude
outputs:
  - path: CLAUDE.md
    template: default
```

- [ ] **Step 2: Write the failing test**

```go
package adapter

import "testing"

func TestLoadBuiltinClaude(t *testing.T) {
	m, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin: %v", err)
	}
	claude, ok := m["claude"]
	if !ok {
		t.Fatalf("claude adapter missing; got keys %v", keys(m))
	}
	if claude.Launch != "claude" || claude.Detect != "claude" {
		t.Fatalf("unexpected manifest: %+v", claude)
	}
	if len(claude.Outputs) != 1 || claude.Outputs[0].Path != "CLAUDE.md" {
		t.Fatalf("unexpected outputs: %+v", claude.Outputs)
	}
}

func TestRenderDefaultTemplate(t *testing.T) {
	m := Manifest{Outputs: []Output{{Path: "CLAUDE.md", Template: "default"}}}
	out := m.Render("hello context")
	if out["CLAUDE.md"] != "hello context" {
		t.Fatalf("want passthrough, got %q", out["CLAUDE.md"])
	}
}

func keys(m map[string]Manifest) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapter/ -v`
Expected: FAIL (undefined: LoadBuiltin / Manifest / Output / Render).

- [ ] **Step 4: Write minimal implementation**

```go
package adapter

import (
	"embed"
	"io/fs"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed builtin/*.yaml
var builtinFS embed.FS

type Output struct {
	Path     string `yaml:"path"`
	Template string `yaml:"template"`
}

type Manifest struct {
	Name    string   `yaml:"name"`
	Detect  string   `yaml:"detect"`
	Launch  string   `yaml:"launch"`
	Outputs []Output `yaml:"outputs"`
}

func LoadBuiltin() (map[string]Manifest, error) {
	out := map[string]Manifest{}
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := fs.ReadFile(builtinFS, filepath.Join("builtin", e.Name()))
		if err != nil {
			return nil, err
		}
		var m Manifest
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		key := strings.TrimSuffix(e.Name(), ".yaml")
		out[key] = m
	}
	return out, nil
}

// Render returns native-file path -> content. Only the "default" template is
// supported in Plan 1; it emits the assembled context verbatim.
func (m Manifest) Render(assembled string) map[string]string {
	out := map[string]string{}
	for _, o := range m.Outputs {
		switch o.Template {
		default: // "default" and unknown templates fall back to passthrough
			out[o.Path] = assembled
		}
	}
	return out
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/adapter/ -v`
Expected: PASS (both tests).

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/
git commit -m "feat: Add declarative YAML adapter with built-in claude manifest"
```

---

### Task 6: `internal/profile` — launch presets

**Files:**
- Create: `internal/profile/profile.go`
- Test: `internal/profile/profile_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Profile struct { Tool, Context string; Skills, Vault []string }` (yaml: `tool`, `context`, `skills`, `vault`)
  - `func Default(tool string) Profile` → `{Tool: tool, Context: "all"}`
  - `func Load(path string) (Profile, error)`
  - `func Save(path string, p Profile) error`

- [ ] **Step 1: Write the failing test**

```go
package profile

import (
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	p := Default("claude")
	if p.Tool != "claude" || p.Context != "all" {
		t.Fatalf("unexpected default: %+v", p)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claude.yaml")
	want := Profile{Tool: "claude", Context: "all", Skills: []string{"review.md"}}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Tool != "claude" || len(got.Skills) != 1 || got.Skills[0] != "review.md" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/profile/ -v`
Expected: FAIL (undefined: Default / Profile / Save / Load).

- [ ] **Step 3: Write minimal implementation**

```go
package profile

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Profile struct {
	Tool    string   `yaml:"tool"`
	Context string   `yaml:"context"` // "all" or explicit context filenames are honored in Plan 2
	Skills  []string `yaml:"skills"`
	Vault   []string `yaml:"vault"`
}

func Default(tool string) Profile {
	return Profile{Tool: tool, Context: "all"}
}

func Load(path string) (Profile, error) {
	var p Profile
	b, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	if err := yaml.Unmarshal(b, &p); err != nil {
		return p, err
	}
	return p, nil
}

func Save(path string, p Profile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/profile/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/profile/
git commit -m "feat: Add profile package for launch presets"
```

---

### Task 7: `internal/sync` — assemble context and write native files

**Files:**
- Create: `internal/sync/sync.go`
- Test: `internal/sync/sync_test.go`

**Interfaces:**
- Consumes: `profile.Profile`, `adapter.Manifest`.
- Produces:
  - `func Assemble(sloopDir string, p profile.Profile) (string, error)` — concatenates `context/*.md` (alphabetical), then `p.Skills` from `skills/`, then `p.Vault` from `vault/`; each file prefixed with `## <filename>\n\n` and separated by a blank line.
  - `func WriteNativeFiles(root string, m adapter.Manifest, assembled string) ([]string, error)` — writes each rendered output under `root`; returns the written paths.

- [ ] **Step 1: Write the failing test**

```go
package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/profile"
)

func TestAssembleOrdersContextThenSkills(t *testing.T) {
	sloopDir := t.TempDir()
	mustWrite(t, filepath.Join(sloopDir, "context", "a.md"), "alpha")
	mustWrite(t, filepath.Join(sloopDir, "context", "b.md"), "bravo")
	mustWrite(t, filepath.Join(sloopDir, "skills", "review.md"), "do a review")

	out, err := Assemble(sloopDir, profile.Profile{Context: "all", Skills: []string{"review.md"}})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	ia := strings.Index(out, "alpha")
	ib := strings.Index(out, "bravo")
	ir := strings.Index(out, "do a review")
	if !(ia < ib && ib < ir) {
		t.Fatalf("wrong order: a=%d b=%d review=%d\n%s", ia, ib, ir, out)
	}
	if !strings.Contains(out, "## a.md") || !strings.Contains(out, "## review.md") {
		t.Fatalf("missing source headings:\n%s", out)
	}
}

func TestWriteNativeFiles(t *testing.T) {
	root := t.TempDir()
	m := adapter.Manifest{Outputs: []adapter.Output{{Path: "CLAUDE.md", Template: "default"}}}
	written, err := WriteNativeFiles(root, m, "hello")
	if err != nil {
		t.Fatalf("WriteNativeFiles: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("want 1 file, got %v", written)
	}
	b, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if string(b) != "hello" {
		t.Fatalf("want hello, got %q", string(b))
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sync/ -v`
Expected: FAIL (undefined: Assemble / WriteNativeFiles).

- [ ] **Step 3: Write minimal implementation**

```go
package sync

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/profile"
)

func Assemble(sloopDir string, p profile.Profile) (string, error) {
	var b strings.Builder

	// 1. all context/*.md, alphabetical
	contextDir := filepath.Join(sloopDir, "context")
	names, err := markdownFiles(contextDir)
	if err != nil {
		return "", err
	}
	for _, name := range names {
		if err := appendFile(&b, contextDir, name); err != nil {
			return "", err
		}
	}

	// 2. selected skills, in listed order
	for _, name := range p.Skills {
		if err := appendFile(&b, filepath.Join(sloopDir, "skills"), name); err != nil {
			return "", err
		}
	}

	// 3. selected vault, in listed order
	for _, name := range p.Vault {
		if err := appendFile(&b, filepath.Join(sloopDir, "vault"), name); err != nil {
			return "", err
		}
	}

	return b.String(), nil
}

func markdownFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func appendFile(b *strings.Builder, dir, name string) error {
	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	b.WriteString("## " + name + "\n\n")
	b.Write(content)
	b.WriteString("\n\n")
	return nil
}

func WriteNativeFiles(root string, m adapter.Manifest, assembled string) ([]string, error) {
	rendered := m.Render(assembled)
	var written []string
	for rel, content := range rendered {
		dest := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
			return nil, err
		}
		written = append(written, dest)
	}
	return written, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sync/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/sync/
git commit -m "feat: Add sync package to assemble context and write native files"
```

---

### Task 8: `internal/runner` — launch behind an interface

**Files:**
- Create: `internal/runner/runner.go`
- Test: `internal/runner/runner_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Spec struct { Dir, Command string; Args []string }`
  - `type Runner interface { Launch(Spec) error }`
  - `type ExecRunner struct{}` implementing `Launch` via `exec.Command` with inherited stdio and `cmd.Dir = Spec.Dir`.
  - `func BuildExecCmd(s Spec) *exec.Cmd` — the testable seam that constructs (but does not run) the command.

- [ ] **Step 1: Write the failing test**

```go
package runner

import "testing"

func TestBuildExecCmdSetsDirAndArgs(t *testing.T) {
	cmd := BuildExecCmd(Spec{Dir: "/tmp/backend", Command: "claude", Args: []string{"--resume"}})
	if cmd.Dir != "/tmp/backend" {
		t.Fatalf("want dir /tmp/backend, got %s", cmd.Dir)
	}
	// cmd.Args[0] is the command name, followed by the args.
	if len(cmd.Args) != 2 || cmd.Args[0] != "claude" || cmd.Args[1] != "--resume" {
		t.Fatalf("unexpected args: %v", cmd.Args)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -v`
Expected: FAIL (undefined: BuildExecCmd / Spec).

- [ ] **Step 3: Write minimal implementation**

```go
package runner

import (
	"os"
	"os/exec"
)

type Spec struct {
	Dir     string
	Command string
	Args    []string
}

type Runner interface {
	Launch(Spec) error
}

func BuildExecCmd(s Spec) *exec.Cmd {
	cmd := exec.Command(s.Command, s.Args...)
	cmd.Dir = s.Dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

type ExecRunner struct{}

func (ExecRunner) Launch(s Spec) error {
	return BuildExecCmd(s).Run()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runner/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/runner/
git commit -m "feat: Add exec runner with testable command builder"
```

---

### Task 9: `sloop init` command

**Files:**
- Create: `internal/cli/commands/init.go` (the existing `init.go` is the command *registry*; do NOT overwrite it — see note)
- Modify: `internal/cli/commands/registry.go` (rename target — see Step 1)
- Test: `internal/cli/commands/initcmd_test.go`

> **Note on the existing `init.go`:** the current `internal/cli/commands/init.go` actually holds the command *registry* (`Register`, `add`, `registry`), not an init command. To avoid confusion, first **rename** it.

**Interfaces:**
- Consumes: `config.SaveProject`, `config.GlobalDBPath`, `profile.Default`, `profile.Save`, `session.Open`/`RegisterWorkspace`, `workspace`.
- Produces:
  - `func RunInit(dir string) error` — testable core: creates `<dir>/.sloop/{context,skills,vault,profiles}`, writes `config.yaml` (`tools: [claude]`, `default_tool: claude`), writes `context/project.md` (starter), writes `profiles/claude.yaml` = `profile.Default("claude")`, writes `.sloop/.gitignore` (ignoring generated files), and registers the workspace in the global DB.
  - `func RegisterInit(cmd *cobra.Command)` — Cobra wiring; registered via `add(RegisterInit)`.

- [ ] **Step 1: Rename the registry file and register the init command**

```bash
git mv internal/cli/commands/init.go internal/cli/commands/registry.go
```

In `internal/cli/commands/registry.go`, update the `init()` to also register the new command:

```go
func init() {
	add(RegisterNew)
	add(RegisterInit)
}
```

- [ ] **Step 2: Write the failing test**

```go
package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunInitScaffolds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir()) // isolate the global DB

	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	for _, p := range []string{
		".sloop/config.yaml",
		".sloop/context/project.md",
		".sloop/profiles/claude.yaml",
		".sloop/.gitignore",
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Fatalf("expected %s: %v", p, err)
		}
	}
	for _, d := range []string{".sloop/skills", ".sloop/vault"} {
		if fi, err := os.Stat(filepath.Join(dir, d)); err != nil || !fi.IsDir() {
			t.Fatalf("expected dir %s: %v", d, err)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/cli/commands/ -run TestRunInitScaffolds -v`
Expected: FAIL (undefined: RunInit).

- [ ] **Step 4: Write minimal implementation**

`internal/cli/commands/init.go`:
```go
package commands

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/profile"
	"github.com/stroops/sloop/internal/session"
)

const starterContext = `# Project Context

Describe this project so AI tools start with the right background.
`

const sloopGitignore = `# Local, machine-specific caches
cache/
*.local
`

func RunInit(dir string) error {
	sloopDir := filepath.Join(dir, config.SloopDirName)
	for _, sub := range []string{"context", "skills", "vault", "profiles"} {
		if err := os.MkdirAll(filepath.Join(sloopDir, sub), 0o755); err != nil {
			return err
		}
	}

	if err := config.SaveProject(sloopDir, &config.Project{
		Tools:       []string{"claude"},
		DefaultTool: "claude",
	}); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(sloopDir, "context", "project.md"),
		[]byte(starterContext), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(sloopDir, ".gitignore"),
		[]byte(sloopGitignore), 0o644); err != nil {
		return err
	}
	if err := profile.Save(filepath.Join(sloopDir, "profiles", "claude.yaml"),
		profile.Default("claude")); err != nil {
		return err
	}

	// Register the workspace in the global DB (best-effort).
	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return err
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	_, err = store.RegisterWorkspace(filepath.Base(abs), abs)
	return err
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold a .sloop workspace in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if err := RunInit(cwd); err != nil {
			return err
		}
		cmd.Printf("⚓ Initialized sloop workspace in %s\n", filepath.Join(cwd, config.SloopDirName))
		return nil
	},
}

func RegisterInit(cmd *cobra.Command) { cmd.AddCommand(initCmd) }
```

> The `.sloop/.gitignore` only ignores local caches inside `.sloop/` itself (valid, self-contained). Ignoring the generated native files at the workspace root requires touching the root `.gitignore`, which Plan 2's `sync` handles safely (create-or-append without clobbering). Not asserted beyond existence here.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/commands/ -run TestRunInitScaffolds -v`
Expected: PASS.

- [ ] **Step 6: Build the binary and smoke-test init**

Run:
```bash
go build -o /tmp/sloop ./cmd/sloop
cd "$(mktemp -d)" && /tmp/sloop init && ls -la .sloop
```
Expected: `.sloop/` with `config.yaml`, `context/`, `skills/`, `vault/`, `profiles/`, `.gitignore`.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/commands/
git commit -m "feat: Add sloop init to scaffold and register a workspace"
```

---

### Task 10: `sloop sync` command

**Files:**
- Create: `internal/cli/commands/sync.go`
- Modify: `internal/cli/commands/registry.go` (`add(RegisterSync)`)
- Test: `internal/cli/commands/synccmd_test.go`

**Interfaces:**
- Consumes: `workspace.Resolve`, `config.LoadProject`, `profile.Load`/`Default`, `adapter.LoadBuiltin`, `sync.Assemble`/`WriteNativeFiles`.
- Produces:
  - `func RunSync(startDir, target string) ([]string, error)` — resolves the workspace from `startDir`, resolves the profile for `target` (a tool or profile name; empty → project `DefaultTool`), assembles context, writes native files, and returns the written paths.
  - `func RegisterSync(cmd *cobra.Command)`
  - Shared helper `func resolveProfile(sloopDir, target, defaultTool string) (profile.Profile, error)` (also used by `run` in Task 11).

- [ ] **Step 1: Write the failing test**

```go
package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSyncWritesClaudeMd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	// Add a distinctive line to context so we can detect it in output.
	ctx := filepath.Join(dir, ".sloop", "context", "project.md")
	if err := os.WriteFile(ctx, []byte("MARKER-CONTEXT"), 0o644); err != nil {
		t.Fatal(err)
	}

	written, err := RunSync(dir, "claude")
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("want 1 written file, got %v", written)
	}
	b, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(b), "MARKER-CONTEXT") {
		t.Fatalf("CLAUDE.md missing context:\n%s", string(b))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands/ -run TestRunSyncWritesClaudeMd -v`
Expected: FAIL (undefined: RunSync).

- [ ] **Step 3: Write minimal implementation**

`internal/cli/commands/sync.go`:
```go
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/profile"
	syncpkg "github.com/stroops/sloop/internal/sync"
	"github.com/stroops/sloop/internal/workspace"
)

func resolveProfile(sloopDir, target, defaultTool string) (profile.Profile, error) {
	if target == "" {
		target = defaultTool
	}
	if target == "" {
		return profile.Profile{}, fmt.Errorf("no target tool or profile given and no default_tool set")
	}
	// A profile file wins over a bare tool name.
	profPath := filepath.Join(sloopDir, "profiles", target+".yaml")
	if _, err := os.Stat(profPath); err == nil {
		return profile.Load(profPath)
	}
	return profile.Default(target), nil
}

// RunSync resolves the workspace + profile and writes native files. Returns the
// written paths.
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
	manifests, err := adapter.LoadBuiltin()
	if err != nil {
		return nil, err
	}
	m, ok := manifests[prof.Tool]
	if !ok {
		return nil, fmt.Errorf("unknown tool %q (no adapter)", prof.Tool)
	}
	assembled, err := syncpkg.Assemble(ws.SloopDir(), prof)
	if err != nil {
		return nil, err
	}
	return syncpkg.WriteNativeFiles(ws.Root, m, assembled)
}

var syncCmd = &cobra.Command{
	Use:   "sync [tool|profile]",
	Short: "Regenerate native context files from .sloop",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		target := ""
		if len(args) == 1 {
			target = args[0]
		}
		written, err := RunSync(cwd, target)
		if err != nil {
			return err
		}
		for _, w := range written {
			cmd.Printf("synced %s\n", w)
		}
		return nil
	},
}

func RegisterSync(cmd *cobra.Command) { cmd.AddCommand(syncCmd) }
```

Add `add(RegisterSync)` to `registry.go`'s `init()`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/commands/ -run TestRunSync -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/commands/
git commit -m "feat: Add sloop sync to render native context files"
```

---

### Task 11: `sloop run` command

**Files:**
- Create: `internal/cli/commands/run.go`
- Modify: `internal/cli/commands/registry.go` (`add(RegisterRun)`)
- Test: `internal/cli/commands/runcmd_test.go`

**Interfaces:**
- Consumes: `RunSync`/`resolveProfile` (Task 10), `workspace.Resolve`, `session` store, `adapter.LoadBuiltin`, `runner.Runner`/`Spec`.
- Produces:
  - `func RunRun(startDir, target string, r runner.Runner) error` — resolves workspace + profile, syncs native files, records a session (best-effort), then launches via `r.Launch(...)` with `Dir = workspace root` and `Command = manifest.Launch`. Records `ended_at` after the launch returns.
  - `func RegisterRun(cmd *cobra.Command)` — wires the Cobra command using `runner.ExecRunner{}`.

- [ ] **Step 1: Write the failing test**

```go
package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stroops/sloop/internal/runner"
)

type fakeRunner struct{ got runner.Spec }

func (f *fakeRunner) Launch(s runner.Spec) error { f.got = s; return nil }

func TestRunRunSyncsAndLaunches(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	fr := &fakeRunner{}
	if err := RunRun(dir, "claude", fr); err != nil {
		t.Fatalf("RunRun: %v", err)
	}

	// Launched claude at the workspace root.
	if fr.got.Command != "claude" {
		t.Fatalf("want command claude, got %q", fr.got.Command)
	}
	wantDir, _ := filepath.Abs(dir)
	gotDir, _ := filepath.Abs(fr.got.Dir)
	if gotDir != wantDir {
		t.Fatalf("want dir %s, got %s", wantDir, gotDir)
	}
	// Sync ran as part of run.
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatalf("expected CLAUDE.md after run: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands/ -run TestRunRunSyncsAndLaunches -v`
Expected: FAIL (undefined: RunRun).

- [ ] **Step 3: Write minimal implementation**

`internal/cli/commands/run.go`:
```go
package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/runner"
	"github.com/stroops/sloop/internal/session"
	"github.com/stroops/sloop/internal/workspace"
)

func RunRun(startDir, target string, r runner.Runner) error {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return err
	}
	proj, err := config.LoadProject(ws.SloopDir())
	if err != nil {
		return err
	}
	prof, err := resolveProfile(ws.SloopDir(), target, proj.DefaultTool)
	if err != nil {
		return err
	}
	manifests, err := adapter.LoadBuiltin()
	if err != nil {
		return err
	}
	m, ok := manifests[prof.Tool]
	if !ok {
		return fmt.Errorf("unknown tool %q (no adapter)", prof.Tool)
	}

	// Sync native files before launch.
	if _, err := RunSync(startDir, target); err != nil {
		return err
	}

	// Record session (best-effort: never block the launch).
	sessID, store := recordSessionBestEffort(ws, prof.Tool, target)
	if store != nil {
		defer store.Close()
	}

	launchErr := r.Launch(runner.Spec{Dir: ws.Root, Command: m.Launch})

	if store != nil && sessID > 0 {
		_ = store.EndSession(sessID, time.Now())
	}
	return launchErr
}

func recordSessionBestEffort(ws *workspace.Workspace, tool, profileName string) (int64, *session.Store) {
	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return 0, nil
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return 0, nil
	}
	w, err := store.RegisterWorkspace(ws.Name, ws.Root)
	if err != nil {
		store.Close()
		return 0, nil
	}
	id, err := store.RecordSession(session.Session{
		WorkspaceID: w.ID, Tool: tool, Profile: profileName, Cwd: ws.Root, StartedAt: time.Now(),
	})
	if err != nil {
		store.Close()
		return 0, nil
	}
	return id, store
}

var runCmd = &cobra.Command{
	Use:   "run [tool|profile]",
	Short: "Sync context and launch an AI tool in the workspace",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		target := ""
		if len(args) == 1 {
			target = args[0]
		}
		return RunRun(cwd, target, runner.ExecRunner{})
	},
}

func RegisterRun(cmd *cobra.Command) { cmd.AddCommand(runCmd) }
```

Add `add(RegisterRun)` to `registry.go`'s `init()`.

- [ ] **Step 4: Run the full command test suite**

Run: `go test ./internal/cli/commands/ -v`
Expected: PASS (init, sync, run tests).

- [ ] **Step 5: Run the whole test suite and build**

Run: `go build ./... && go test ./...`
Expected: build clean, all packages PASS.

- [ ] **Step 6: End-to-end smoke test**

Run:
```bash
go build -o /tmp/sloop ./cmd/sloop
WORK="$(mktemp -d)" && cd "$WORK" && /tmp/sloop init
printf '# Project Context\n\nThis is the acme service.\n' > .sloop/context/project.md
/tmp/sloop sync claude && cat CLAUDE.md
```
Expected: `CLAUDE.md` contains `## project.md` and the "acme service" text. (`run` itself launches the real `claude` binary, so only test `run` interactively if `claude` is installed.)

- [ ] **Step 7: Commit**

```bash
git add internal/cli/commands/
git commit -m "feat: Add sloop run to sync, record session, and launch the tool"
```

---

## Self-Review

**Spec coverage (Plan 1 portion):**
- §2.1 cleanup (delete sloopd, remove `--socket`, fix store) → Task 1 ✓
- §4 storage layering (`.sloop/` vs `~/.sloop/sloop.db`) → Tasks 2, 3, 9 ✓
- §3 workspace resolution (walk-up) → Task 4 ✓ (registry `-w` lookup is wired in Plan 2's `run` flag; the store method `WorkspaceByName` exists here)
- §5 context delivery (assemble → render → write) → Tasks 5, 7, 10 ✓
- §6 declarative YAML adapter (built-in claude) → Task 5 ✓
- §3 profile shape + default profile → Task 6 ✓
- §7 SQLite sessions → Task 3, recorded in Task 11 ✓
- §12 commands `init`/`sync`/`run` → Tasks 9, 10, 11 ✓

**Deferred to Plan 2 (explicitly not in this plan):** tmux runner branch (§8), detection + auto-enable (§9), interaction mode (§10), `status` (§11), `ls`/`attach`/`tools`/`doctor`/`skill` (§12), Cursor/Aider adapters (§6), `-w` flag surface on `run`/`sync`, mtime freshness. The `run` flow here uses `ExecRunner` only.

**Placeholder scan:** No TBD/TODO. The `.gitignore` caveat in Task 9 is a documented known-limitation with a clear Plan 2 follow-up, not a placeholder; it is not asserted by any test.

**Type consistency:** `RunSync(startDir, target string) ([]string, error)`, `resolveProfile(sloopDir, target, defaultTool string)`, `RunRun(startDir, target string, r runner.Runner)`, `runner.Spec{Dir, Command, Args}`, `session.Session{WorkspaceID, Tool, Profile, Cwd, StartedAt, EndedAt}` are used identically across Tasks 7–11. `profile.Profile` fields (`Tool`, `Context`, `Skills`, `Vault`) match between Tasks 6, 7, 10.
