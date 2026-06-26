# Sloop Plan 2 — Workflow Features Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the v1 feature set on top of Plan 1's core: on-demand tool/tmux detection, detection-driven `init` auto-enable, optional tmux multiplexing with plain-exec fallback, the `ask`/`auto` interaction mode, a Cursor built-in adapter plus user-supplied adapters, the `-w` workspace flag, and the `status`/`ls`/`attach`/`tools`/`doctor`/`skill` commands.

**Architecture:** Still a single `sloop` binary, no daemon. New leaf package `internal/detect` scans installed tools (via `exec.LookPath`) and tmux. `internal/runner` gains a tmux-aware launcher selected at runtime. `internal/config` gains a global config (`~/.sloop/config.yaml`) holding the interaction `mode`. Commands compose these. Adapters stay declarative YAML; `adapter.Load()` overlays `~/.sloop/adapters/*.yaml` on the embedded built-ins.

**Tech Stack:** Go 1.26, Cobra, `modernc.org/sqlite`, `gopkg.in/yaml.v3`, `go:embed`, `os/exec`.

## Global Constraints

- Builds on Plan 1 (branch `design/sloop-mvp`); all Plan 1 packages and the `init`/`sync`/`run` commands exist and pass tests.
- Single binary `sloop`; no daemon, no gRPC, no network.
- SQLite: `modernc.org/sqlite` only. YAML: `gopkg.in/yaml.v3` only.
- tmux is an **optional enhancement, never a hard dependency**: when absent, every flow falls back to plain `exec` in the current terminal.
- Detection is **on-demand only** (no daemon, no background scan, no cache).
- Safety boundary (hard rule): Sloop **never installs software or runs a package manager**. Auto-enable only ever creates config/profile files in the project.
- Interaction mode resolution precedence: `--auto`/`-y` flag → project `mode` → global `mode` → default `ask`. `--no-input` makes any prompt an error instead of blocking.
- Aider is OUT of scope for Plan 2 (added later as a YAML manifest).
- Cursor built-in manifest: `detect`/`launch`: `agent` (confirmed by the user; `agent --version` works on their machine); output `AGENTS.md` — declarative, trivially editable.
- Commit subjects: `type: Capitalized subject` (type ∈ feat/fix/refactor/docs/chore/perf/revert), no trailing period, no `Co-Authored-By` trailer.
- Do not run `go mod tidy` (no new modules are added). Use `go build ./...` and focused `go test`.
- Spec of record: `docs/superpowers/specs/2026-06-26-sloop-mvp-design.md`.

---

### Task 1: Global config and interaction mode

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/global_test.go`

**Interfaces:**
- Consumes: existing `config.GlobalDir()`.
- Produces:
  - `const ModeAsk = "ask"`, `const ModeAuto = "auto"`
  - `type Global struct { Mode string }` (yaml: `mode`)
  - `func GlobalConfigPath() (string, error)` → `~/.sloop/config.yaml`
  - `func LoadGlobal() (*Global, error)` — if the file is missing, returns `&Global{Mode: ModeAsk}, nil`
  - `func SaveGlobal(g *Global) error`
  - new field on existing `Project`: `Mode string` (yaml: `mode,omitempty`) — optional per-project override

- [ ] **Step 1: Write the failing test**

```go
package config

import "testing"

func TestLoadGlobalDefaultsToAsk(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	g, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if g.Mode != ModeAsk {
		t.Fatalf("want default mode %q, got %q", ModeAsk, g.Mode)
	}
}

func TestSaveAndLoadGlobal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SaveGlobal(&Global{Mode: ModeAuto}); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	g, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if g.Mode != ModeAuto {
		t.Fatalf("want %q, got %q", ModeAuto, g.Mode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoadGlobal -v`
Expected: FAIL (undefined: LoadGlobal / Global / ModeAsk).

- [ ] **Step 3: Write minimal implementation**

Add to `internal/config/config.go` (keep existing code):

```go
const (
	ModeAsk  = "ask"
	ModeAuto = "auto"
)

// Global is the machine-local config at ~/.sloop/config.yaml.
type Global struct {
	Mode string `yaml:"mode"`
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
		return &Global{Mode: ModeAsk}, nil
	}
	if err != nil {
		return nil, err
	}
	var g Global
	if err := yaml.Unmarshal(b, &g); err != nil {
		return nil, err
	}
	if g.Mode == "" {
		g.Mode = ModeAsk
	}
	return &g, nil
}

func SaveGlobal(g *Global) error {
	dir, err := GlobalDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yaml")
	b, err := yaml.Marshal(g)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
```

Also add the `Mode` field to the existing `Project` struct:

```go
type Project struct {
	Tools       []string `yaml:"tools"`
	DefaultTool string   `yaml:"default_tool"`
	Mode        string   `yaml:"mode,omitempty"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all config tests, including the Plan 1 ones).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: Add global config with interaction mode"
```

---

### Task 2: `internal/detect` — on-demand tool and tmux detection

**Files:**
- Create: `internal/detect/detect.go`
- Test: `internal/detect/detect_test.go`

**Interfaces:**
- Consumes: `adapter.Manifest`.
- Produces:
  - `type ToolStatus struct { Key, Name, Binary string; Installed bool; Version string }`
  - `func Tools(manifests map[string]adapter.Manifest) []ToolStatus` — for each manifest, `exec.LookPath(Detect)`; best-effort version; sorted by Key.
  - `type TmuxStatus struct { Installed bool; Version string }`
  - `func Tmux() TmuxStatus`
  - `func InstalledKeys(manifests map[string]adapter.Manifest) []string` — keys whose binary is on PATH, sorted.

- [ ] **Step 1: Write the failing test**

```go
package detect

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stroops/sloop/internal/adapter"
)

func TestToolsDetectsBinaryOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake-binary PATH trick is unix-only")
	}
	dir := t.TempDir()
	// Create a fake executable named "faketool".
	bin := filepath.Join(dir, "faketool")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho v1.2.3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	manifests := map[string]adapter.Manifest{
		"faketool": {Name: "Fake Tool", Detect: "faketool", Launch: "faketool"},
		"missing":  {Name: "Missing", Detect: "definitely-not-installed-xyz", Launch: "x"},
	}
	got := Tools(manifests)
	byKey := map[string]ToolStatus{}
	for _, s := range got {
		byKey[s.Key] = s
	}
	if !byKey["faketool"].Installed {
		t.Fatalf("faketool should be installed: %+v", byKey["faketool"])
	}
	if byKey["missing"].Installed {
		t.Fatalf("missing should not be installed: %+v", byKey["missing"])
	}
}

func TestInstalledKeys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "faketool"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	keys := InstalledKeys(map[string]adapter.Manifest{
		"faketool": {Detect: "faketool"},
		"missing":  {Detect: "nope-xyz"},
	})
	if len(keys) != 1 || keys[0] != "faketool" {
		t.Fatalf("want [faketool], got %v", keys)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/detect/ -v`
Expected: FAIL (undefined: Tools / ToolStatus / InstalledKeys).

- [ ] **Step 3: Write minimal implementation**

```go
package detect

import (
	"context"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/stroops/sloop/internal/adapter"
)

type ToolStatus struct {
	Key       string
	Name      string
	Binary    string
	Installed bool
	Version   string
}

func Tools(manifests map[string]adapter.Manifest) []ToolStatus {
	var out []ToolStatus
	for key, m := range manifests {
		s := ToolStatus{Key: key, Name: m.Name, Binary: m.Detect}
		if _, err := exec.LookPath(m.Detect); err == nil {
			s.Installed = true
			s.Version = bestEffortVersion(m.Detect)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func InstalledKeys(manifests map[string]adapter.Manifest) []string {
	var keys []string
	for key, m := range manifests {
		if _, err := exec.LookPath(m.Detect); err == nil {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

type TmuxStatus struct {
	Installed bool
	Version   string
}

func Tmux() TmuxStatus {
	if _, err := exec.LookPath("tmux"); err != nil {
		return TmuxStatus{}
	}
	return TmuxStatus{Installed: true, Version: bestEffortVersion2("tmux", "-V")}
}

// bestEffortVersion runs `<bin> --version` with a short timeout, returning the
// first line of output or "" on any failure.
func bestEffortVersion(bin string) string {
	return bestEffortVersion2(bin, "--version")
}

func bestEffortVersion2(bin, flag string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, flag).Output()
	if err != nil {
		return ""
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return strings.TrimSpace(line)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/detect/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/detect/
git commit -m "feat: Add on-demand tool and tmux detection"
```

---

### Task 3: Cursor built-in adapter and user-adapter loading

**Files:**
- Create: `internal/adapter/builtin/cursor.yaml`
- Modify: `internal/adapter/adapter.go` (add `Load()` overlaying user adapters)
- Test: `internal/adapter/load_test.go`

**Interfaces:**
- Consumes: `config.UserAdaptersDir()`.
- Produces:
  - `func Load() (map[string]Manifest, error)` — built-ins first, then overlay every `*.yaml` in `~/.sloop/adapters/` (a user file with the same key overrides the built-in). Missing dir is not an error.

- [ ] **Step 1: Create the Cursor manifest**

`internal/adapter/builtin/cursor.yaml`:
```yaml
name: Cursor CLI
detect: agent
launch: agent
outputs:
  - path: AGENTS.md
    template: default
```

- [ ] **Step 2: Write the failing test**

```go
package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIncludesBuiltinCursor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := m["claude"]; !ok {
		t.Fatalf("claude built-in missing")
	}
	cursor, ok := m["cursor"]
	if !ok {
		t.Fatalf("cursor built-in missing")
	}
	if cursor.Launch != "agent" || len(cursor.Outputs) != 1 || cursor.Outputs[0].Path != "AGENTS.md" {
		t.Fatalf("unexpected cursor manifest: %+v", cursor)
	}
}

func TestLoadUserAdapterOverridesBuiltin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	adaptersDir := filepath.Join(home, ".sloop", "adapters")
	if err := os.MkdirAll(adaptersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Override claude's launch command via a user manifest.
	custom := "name: My Claude\ndetect: claude\nlaunch: claude-custom\noutputs:\n  - path: CLAUDE.md\n    template: default\n"
	if err := os.WriteFile(filepath.Join(adaptersDir, "claude.yaml"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m["claude"].Launch != "claude-custom" {
		t.Fatalf("user override not applied: %+v", m["claude"])
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapter/ -run TestLoad -v`
Expected: FAIL (undefined: Load).

- [ ] **Step 4: Write minimal implementation**

Add to `internal/adapter/adapter.go` (keep existing `LoadBuiltin`/`Render`). Add the config import:

```go
import (
	// ...existing imports...
	"os"

	"github.com/stroops/sloop/internal/config"
)

// Load returns built-in manifests overlaid with any user manifests in
// ~/.sloop/adapters/*.yaml (same key overrides the built-in).
func Load() (map[string]Manifest, error) {
	out, err := LoadBuiltin()
	if err != nil {
		return nil, err
	}
	dir, err := config.UserAdaptersDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
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
```

> Note: `filepath`, `strings`, and `yaml` are already imported by adapter.go from Plan 1; only add `os` and the `config` import.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/adapter/ -v`
Expected: PASS (built-in claude + cursor, and user-override tests).

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/
git commit -m "feat: Add Cursor built-in adapter and user adapter loading"
```

---

### Task 4: tmux runner and `attach` command

**Files:**
- Create: `internal/runner/tmux.go`
- Create: `internal/runner/tmux_test.go`
- Create: `internal/cli/commands/attach.go`
- Modify: `internal/cli/commands/registry.go` (`add(RegisterAttach)`)
- Test: `internal/cli/commands/attachcmd_test.go`

**Interfaces:**
- Consumes: `runner.Spec` (Plan 1).
- Produces:
  - `func TmuxAvailable() bool`
  - `func TmuxSessionName(workspace, tool string) string` — sanitize to `[A-Za-z0-9_]`, join `workspace + "__" + tool`
  - `func BuildTmuxNewArgs(session string, s Spec) []string` — `new-session -A -s <session> -c <dir> <command> <args...>`
  - `func BuildTmuxAttachArgs(session string) []string` — `attach -t <session>`
  - `type TmuxRunner struct { Session string }` implementing `Launch(Spec) error` via `exec` of `tmux` with inherited stdio
  - command `sloop attach <name>` → `RunAttach(name string) error` (execs `tmux attach -t <name>`)

- [ ] **Step 1: Write the failing test (tmux args + name)**

`internal/runner/tmux_test.go`:
```go
package runner

import (
	"reflect"
	"testing"
)

func TestTmuxSessionNameSanitizes(t *testing.T) {
	got := TmuxSessionName("my-app", "claude")
	if got != "my_app__claude" {
		t.Fatalf("want my_app__claude, got %s", got)
	}
}

func TestBuildTmuxNewArgs(t *testing.T) {
	args := BuildTmuxNewArgs("backend__claude", Spec{Dir: "/tmp/backend", Command: "claude", Args: []string{"--resume"}})
	want := []string{"new-session", "-A", "-s", "backend__claude", "-c", "/tmp/backend", "claude", "--resume"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("want %v, got %v", want, args)
	}
}

func TestBuildTmuxAttachArgs(t *testing.T) {
	args := BuildTmuxAttachArgs("backend__claude")
	want := []string{"attach", "-t", "backend__claude"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("want %v, got %v", want, args)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run Tmux -v`
Expected: FAIL (undefined: TmuxSessionName / BuildTmuxNewArgs / BuildTmuxAttachArgs).

- [ ] **Step 3: Write the runner implementation**

`internal/runner/tmux.go`:
```go
package runner

import (
	"os"
	"os/exec"
	"strings"
)

func TmuxAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func TmuxSessionName(workspace, tool string) string {
	return sanitize(workspace) + "__" + sanitize(tool)
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// BuildTmuxNewArgs builds `tmux new-session -A -s <session> -c <dir> <command> <args...>`.
// -A attaches if the session already exists, otherwise creates it.
func BuildTmuxNewArgs(session string, s Spec) []string {
	args := []string{"new-session", "-A", "-s", session, "-c", s.Dir, s.Command}
	return append(args, s.Args...)
}

func BuildTmuxAttachArgs(session string) []string {
	return []string{"attach", "-t", session}
}

type TmuxRunner struct {
	Session string
}

func (r TmuxRunner) Launch(s Spec) error {
	cmd := exec.Command("tmux", BuildTmuxNewArgs(r.Session, s)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

- [ ] **Step 4: Run the runner tests**

Run: `go test ./internal/runner/ -v`
Expected: PASS (Plan 1 exec test + new tmux tests).

- [ ] **Step 5: Write the failing test for the attach command**

`internal/cli/commands/attachcmd_test.go`:
```go
package commands

import "testing"

func TestAttachArgsBuilt(t *testing.T) {
	// RunAttach builds the tmux attach args for the given session name.
	if got := attachArgs("backend__claude"); got != "attach -t backend__claude" {
		t.Fatalf("unexpected: %s", got)
	}
}
```

- [ ] **Step 6: Write the attach command**

`internal/cli/commands/attach.go`:
```go
package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/runner"
)

// attachArgs is a tiny testable seam returning the tmux args as a string.
func attachArgs(session string) string {
	return strings.Join(runner.BuildTmuxAttachArgs(session), " ")
}

func RunAttach(session string) error {
	if !runner.TmuxAvailable() {
		return fmt.Errorf("tmux is not installed; attach requires tmux")
	}
	cmd := exec.Command("tmux", runner.BuildTmuxAttachArgs(session)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var attachCmd = &cobra.Command{
	Use:   "attach <session>",
	Short: "Attach to a tmux session created by sloop",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunAttach(args[0])
	},
}

func RegisterAttach(cmd *cobra.Command) { cmd.AddCommand(attachCmd) }
```

Add `add(RegisterAttach)` to `registry.go`'s `init()`.

- [ ] **Step 7: Run command tests + build**

Run: `go test ./internal/cli/commands/ -run TestAttach -v && go build ./...`
Expected: PASS, build OK.

- [ ] **Step 8: Commit**

```bash
git add internal/runner/ internal/cli/commands/
git commit -m "feat: Add tmux runner and attach command"
```

---

### Task 5: Interaction helper (mode-aware confirm)

**Files:**
- Create: `internal/cli/commands/interaction.go`
- Modify: `internal/cli/root.go` (add `--auto`/`-y` and `--no-input` persistent flags)
- Test: `internal/cli/commands/interaction_test.go`

**Interfaces:**
- Consumes: `config.LoadGlobal`, `config.LoadProject`, `config.ModeAuto`.
- Produces:
  - `type Interaction struct { Auto, NoInput bool }`
  - `func ResolveInteraction(projectMode, globalMode string, autoFlag, noInput bool) Interaction` — `Auto = autoFlag || effectiveMode==ModeAuto`, where effectiveMode = first non-empty of projectMode, globalMode.
  - `func (i Interaction) Confirm(prompt string, in io.Reader, out io.Writer) (bool, error)` — `Auto` → true; `NoInput` → error; else read a line, true on `y`/`yes` (case-insensitive).

- [ ] **Step 1: Write the failing test**

```go
package commands

import (
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/config"
)

func TestResolveInteractionPrecedence(t *testing.T) {
	// Flag wins.
	if !ResolveInteraction("", "", true, false).Auto {
		t.Fatal("flag should force auto")
	}
	// Project mode auto.
	if !ResolveInteraction(config.ModeAuto, "", false, false).Auto {
		t.Fatal("project mode auto should win")
	}
	// Global mode auto when project empty.
	if !ResolveInteraction("", config.ModeAuto, false, false).Auto {
		t.Fatal("global mode auto should apply")
	}
	// Default ask.
	if ResolveInteraction("", "", false, false).Auto {
		t.Fatal("default should not be auto")
	}
}

func TestConfirmAutoAndNoInput(t *testing.T) {
	ok, err := Interaction{Auto: true}.Confirm("go?", strings.NewReader(""), &strings.Builder{})
	if err != nil || !ok {
		t.Fatalf("auto should confirm true: ok=%v err=%v", ok, err)
	}
	if _, err := (Interaction{NoInput: true}).Confirm("go?", strings.NewReader(""), &strings.Builder{}); err == nil {
		t.Fatal("no-input should error instead of prompting")
	}
}

func TestConfirmReadsYes(t *testing.T) {
	ok, err := Interaction{}.Confirm("go?", strings.NewReader("y\n"), &strings.Builder{})
	if err != nil || !ok {
		t.Fatalf("want true on y: ok=%v err=%v", ok, err)
	}
	ok, _ = Interaction{}.Confirm("go?", strings.NewReader("n\n"), &strings.Builder{})
	if ok {
		t.Fatal("want false on n")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands/ -run 'TestResolveInteraction|TestConfirm' -v`
Expected: FAIL (undefined: ResolveInteraction / Interaction).

- [ ] **Step 3: Write minimal implementation**

`internal/cli/commands/interaction.go`:
```go
package commands

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/stroops/sloop/internal/config"
)

type Interaction struct {
	Auto    bool
	NoInput bool
}

func ResolveInteraction(projectMode, globalMode string, autoFlag, noInput bool) Interaction {
	effective := projectMode
	if effective == "" {
		effective = globalMode
	}
	return Interaction{
		Auto:    autoFlag || effective == config.ModeAuto,
		NoInput: noInput,
	}
}

func (i Interaction) Confirm(prompt string, in io.Reader, out io.Writer) (bool, error) {
	if i.Auto {
		return true, nil
	}
	if i.NoInput {
		return false, fmt.Errorf("%s (refusing to prompt under --no-input)", prompt)
	}
	fmt.Fprintf(out, "%s [y/N]: ", prompt)
	line, _ := bufio.NewReader(in).ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes", nil
}
```

- [ ] **Step 4: Add the persistent flags**

In `internal/cli/root.go` `func init()`, add:
```go
	rootCmd.PersistentFlags().BoolP("auto", "y", false, "assume yes / run automatically without prompts")
	rootCmd.PersistentFlags().Bool("no-input", false, "never prompt; fail instead of asking")
```

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/cli/commands/ -run 'TestResolveInteraction|TestConfirm' -v && go build ./...`
Expected: PASS, build OK.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/commands/interaction.go internal/cli/root.go
git commit -m "feat: Add mode-aware interaction helper and global flags"
```

---

### Task 6: `tools` and `doctor` commands

**Files:**
- Create: `internal/cli/commands/tools.go`
- Create: `internal/cli/commands/doctor.go`
- Modify: `internal/cli/commands/registry.go` (`add(RegisterTools)`, `add(RegisterDoctor)`)
- Test: `internal/cli/commands/toolsdoctor_test.go`

**Interfaces:**
- Consumes: `adapter.Load`, `detect.Tools`, `detect.Tmux`, `config.LoadGlobal`.
- Produces:
  - `func RunTools(w io.Writer) error` — lists each adapter with installed/missing + version.
  - `func RunDoctor(w io.Writer) error` — tools section + tmux line + global mode line.

- [ ] **Step 1: Write the failing test**

```go
package commands

import (
	"strings"
	"testing"
)

func TestRunToolsListsClaude(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var b strings.Builder
	if err := RunTools(&b); err != nil {
		t.Fatalf("RunTools: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "claude") || !strings.Contains(out, "cursor") {
		t.Fatalf("tools output missing adapters:\n%s", out)
	}
}

func TestRunDoctorReportsTmuxAndMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var b strings.Builder
	if err := RunDoctor(&b); err != nil {
		t.Fatalf("RunDoctor: %v", err)
	}
	out := strings.ToLower(b.String())
	if !strings.Contains(out, "tmux") || !strings.Contains(out, "mode") {
		t.Fatalf("doctor output missing tmux/mode:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands/ -run 'TestRunTools|TestRunDoctor' -v`
Expected: FAIL (undefined: RunTools / RunDoctor).

- [ ] **Step 3: Write minimal implementation**

`internal/cli/commands/tools.go`:
```go
package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/detect"
)

func RunTools(w io.Writer) error {
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	for _, s := range detect.Tools(manifests) {
		status := "missing"
		if s.Installed {
			status = "installed"
			if s.Version != "" {
				status += " (" + s.Version + ")"
			}
		}
		fmt.Fprintf(w, "%-10s %-14s %s\n", s.Key, s.Name, status)
	}
	return nil
}

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List configured AI tool adapters and their install status",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunTools(cmd.OutOrStdout())
	},
}

func RegisterTools(cmd *cobra.Command) { cmd.AddCommand(toolsCmd) }
```

`internal/cli/commands/doctor.go`:
```go
package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/detect"
)

func RunDoctor(w io.Writer) error {
	fmt.Fprintln(w, "Tools:")
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	for _, s := range detect.Tools(manifests) {
		mark := "✗"
		extra := ""
		if s.Installed {
			mark = "✓"
			if s.Version != "" {
				extra = " " + s.Version
			}
		}
		fmt.Fprintf(w, "  %s %s%s\n", mark, s.Key, extra)
	}

	tmux := detect.Tmux()
	mark := "✗ (optional — exec fallback in use)"
	if tmux.Installed {
		mark = "✓ " + tmux.Version
	}
	fmt.Fprintf(w, "tmux: %s\n", mark)

	g, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "mode: %s\n", g.Mode)
	return nil
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the sloop environment (tools, tmux, config)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunDoctor(cmd.OutOrStdout())
	},
}

func RegisterDoctor(cmd *cobra.Command) { cmd.AddCommand(doctorCmd) }
```

Add `add(RegisterTools)` and `add(RegisterDoctor)` to `registry.go`'s `init()`.

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/cli/commands/ -run 'TestRunTools|TestRunDoctor' -v && go build ./...`
Expected: PASS, build OK.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/commands/
git commit -m "feat: Add tools and doctor commands"
```

---

### Task 7: Sync freshness + `status` and `ls` commands

**Files:**
- Create: `internal/sync/freshness.go`
- Create: `internal/sync/freshness_test.go`
- Create: `internal/cli/commands/status.go`
- Create: `internal/cli/commands/ls.go`
- Modify: `internal/cli/commands/registry.go` (`add(RegisterStatus)`, `add(RegisterLs)`)
- Test: `internal/cli/commands/statusls_test.go`

**Interfaces:**
- Consumes: `workspace.Resolve`, `config.LoadProject`, `adapter.Load`, `profile`, `session`, `sync`.
- Produces:
  - `func Stale(root, sloopDir string, m adapter.Manifest, p profile.Profile) (bool, error)` — true if any selected source file's mtime is newer than the oldest generated native output, or an output is missing.
  - `func RunStatus(startDir string, w io.Writer) error` — one line: `⚓ <workspace> · <tool> · sync:<fresh|stale>` (tool = project DefaultTool).
  - `func RunLs(w io.Writer) error` — lists registered workspaces and recent sessions.

- [ ] **Step 1: Write the failing freshness test**

`internal/sync/freshness_test.go`:
```go
package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/profile"
)

func TestStaleWhenOutputMissing(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	mustWrite(t, filepath.Join(sloopDir, "context", "a.md"), "x")
	m := adapter.Manifest{Outputs: []adapter.Output{{Path: "CLAUDE.md", Template: "default"}}}
	stale, err := Stale(root, sloopDir, m, profile.Profile{Context: "all"})
	if err != nil {
		t.Fatalf("Stale: %v", err)
	}
	if !stale {
		t.Fatal("missing output should be stale")
	}
}

func TestFreshAfterWrite(t *testing.T) {
	root := t.TempDir()
	sloopDir := filepath.Join(root, ".sloop")
	mustWrite(t, filepath.Join(sloopDir, "context", "a.md"), "x")
	// Generate the output after the source.
	time.Sleep(10 * time.Millisecond)
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), "rendered")
	m := adapter.Manifest{Outputs: []adapter.Output{{Path: "CLAUDE.md", Template: "default"}}}
	stale, err := Stale(root, sloopDir, m, profile.Profile{Context: "all"})
	if err != nil {
		t.Fatalf("Stale: %v", err)
	}
	if stale {
		t.Fatal("output newer than source should be fresh")
	}
}
```

(`mustWrite` already exists in the Plan 1 `sync_test.go`, same package.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sync/ -run 'TestStale|TestFresh' -v`
Expected: FAIL (undefined: Stale).

- [ ] **Step 3: Write the freshness implementation**

`internal/sync/freshness.go`:
```go
package sync

import (
	"os"
	"path/filepath"

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

func newestSourceMtime(sloopDir string, p profile.Profile) (t timeType, err error) {
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
```

> Replace `timeType` with `time.Time` and add `"time"` to the imports. (`timeType` is written here only to flag the spot — the implementer must use `time.Time` and import `time`.)

- [ ] **Step 4: Run the freshness tests**

Run: `go test ./internal/sync/ -v`
Expected: PASS (Plan 1 sync tests + freshness).

- [ ] **Step 5: Write the failing status/ls test**

`internal/cli/commands/statusls_test.go`:
```go
package commands

import (
	"strings"
	"testing"
)

func TestRunStatusShowsWorkspaceAndStale(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	var b strings.Builder
	if err := RunStatus(dir, &b); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, filepathBase(dir)) || !strings.Contains(out, "sync:") {
		t.Fatalf("status missing workspace/sync:\n%s", out)
	}
}

func TestRunLsListsWorkspace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	var b strings.Builder
	if err := RunLs(&b); err != nil {
		t.Fatalf("RunLs: %v", err)
	}
	if !strings.Contains(b.String(), filepathBase(dir)) {
		t.Fatalf("ls missing workspace:\n%s", b.String())
	}
}
```

Add this helper at the bottom of the test file:
```go
func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}
```

- [ ] **Step 6: Write status and ls commands**

`internal/cli/commands/status.go`:
```go
package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/config"
	syncpkg "github.com/stroops/sloop/internal/sync"
	"github.com/stroops/sloop/internal/workspace"
)

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
	prof, err := resolveProfile(ws.SloopDir(), tool, proj.DefaultTool)
	if err != nil {
		return err
	}
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	sync := "unknown"
	if m, ok := manifests[prof.Tool]; ok {
		stale, err := syncpkg.Stale(ws.Root, ws.SloopDir(), m, prof)
		if err != nil {
			return err
		}
		if stale {
			sync = "stale"
		} else {
			sync = "fresh"
		}
	}
	fmt.Fprintf(w, "⚓ %s · %s · sync:%s\n", ws.Name, tool, sync)
	return nil
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current sloop workspace status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return RunStatus(cwd, cmd.OutOrStdout())
	},
}

func RegisterStatus(cmd *cobra.Command) { cmd.AddCommand(statusCmd) }
```

`internal/cli/commands/ls.go`:
```go
package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/session"
)

func RunLs(w io.Writer) error {
	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return err
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	workspaces, err := store.ListWorkspaces()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "Workspaces:")
	for _, ws := range workspaces {
		fmt.Fprintf(w, "  %-16s %s\n", ws.Name, ws.Path)
	}

	sessions, err := store.ListSessions(10)
	if err != nil {
		return err
	}
	if len(sessions) > 0 {
		fmt.Fprintln(w, "Recent sessions:")
		for _, s := range sessions {
			fmt.Fprintf(w, "  %s  tool=%s  %s\n", s.StartedAt.Format("2006-01-02 15:04"), s.Tool, s.Cwd)
		}
	}
	return nil
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List sloop workspaces and recent sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunLs(cmd.OutOrStdout())
	},
}

func RegisterLs(cmd *cobra.Command) { cmd.AddCommand(lsCmd) }
```

Add `add(RegisterStatus)` and `add(RegisterLs)` to `registry.go`'s `init()`.

- [ ] **Step 7: Run tests + build**

Run: `go test ./internal/sync/ ./internal/cli/commands/ -run 'TestStale|TestFresh|TestRunStatus|TestRunLs' -v && go build ./...`
Expected: PASS, build OK.

- [ ] **Step 8: Commit**

```bash
git add internal/sync/ internal/cli/commands/
git commit -m "feat: Add sync freshness check and status and ls commands"
```

---

### Task 8: `skill new` command

**Files:**
- Create: `internal/cli/commands/skill.go`
- Modify: `internal/cli/commands/registry.go` (`add(RegisterSkill)`)
- Test: `internal/cli/commands/skillcmd_test.go`

**Interfaces:**
- Consumes: `workspace.Resolve`.
- Produces:
  - `func RunSkillNew(startDir, name string) (string, error)` — creates `<sloopDir>/skills/<name>.md` with a starter template; returns the created path; errors if it already exists.

- [ ] **Step 1: Write the failing test**

```go
package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunSkillNewCreatesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	path, err := RunSkillNew(dir, "review")
	if err != nil {
		t.Fatalf("RunSkillNew: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("skill file not created: %v", err)
	}
	if filepath.Base(path) != "review.md" {
		t.Fatalf("want review.md, got %s", path)
	}
	// Creating again should error.
	if _, err := RunSkillNew(dir, "review"); err == nil {
		t.Fatal("expected error creating duplicate skill")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands/ -run TestRunSkillNew -v`
Expected: FAIL (undefined: RunSkillNew).

- [ ] **Step 3: Write minimal implementation**

`internal/cli/commands/skill.go`:
```go
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/workspace"
)

func RunSkillNew(startDir, name string) (string, error) {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(ws.SloopDir(), "skills", name+".md")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("skill %q already exists at %s", name, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	body := fmt.Sprintf("# %s\n\nDescribe the reusable workflow or prompt for %q here.\n", name, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage reusable skills",
}

var skillNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Scaffold a new skill file under .sloop/skills",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		path, err := RunSkillNew(cwd, args[0])
		if err != nil {
			return err
		}
		cmd.Printf("created %s\n", path)
		return nil
	},
}

func RegisterSkill(cmd *cobra.Command) {
	skillCmd.AddCommand(skillNewCmd)
	cmd.AddCommand(skillCmd)
}
```

Add `add(RegisterSkill)` to `registry.go`'s `init()`.

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/cli/commands/ -run TestRunSkillNew -v && go build ./...`
Expected: PASS, build OK.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/commands/
git commit -m "feat: Add skill new command"
```

---

### Task 9: Detection-driven `init` auto-enable

**Files:**
- Modify: `internal/cli/commands/init.go`
- Test: `internal/cli/commands/initcmd_test.go` (extend)

**Interfaces:**
- Consumes: `detect.InstalledKeys`, `adapter.Load`.
- Produces:
  - Updated `RunInit(dir string) error`: detects installed known tools and enables them in `config.yaml` + creates a `profiles/<tool>.yaml` per enabled tool. If no known tool is detected, falls back to enabling `claude` so the workspace is always usable. `DefaultTool` = `claude` if enabled, else the first enabled tool.

- [ ] **Step 1: Write the failing test (fallback when nothing detected)**

Add to `initcmd_test.go`:
```go
func TestRunInitFallsBackToClaude(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // empty PATH: no tools detected
	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sloop", "profiles", "claude.yaml")); err != nil {
		t.Fatalf("expected claude fallback profile: %v", err)
	}
	p, err := config.LoadProject(filepath.Join(dir, ".sloop"))
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if p.DefaultTool != "claude" {
		t.Fatalf("want default claude, got %q", p.DefaultTool)
	}
}
```

Add `"github.com/stroops/sloop/internal/config"` to the test imports if not present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands/ -run TestRunInitFallsBackToClaude -v`
Expected: FAIL — until init uses detection, this passes trivially; to make it a real RED, the implementer should first confirm the test compiles and then refactor `RunInit`. Because Plan 1's `RunInit` already hardcodes claude, this test passes immediately; treat this task as a refactor verified by BOTH the new test and the existing `TestRunInitScaffolds` continuing to pass.

- [ ] **Step 3: Update the implementation**

Replace the body of `RunInit` in `internal/cli/commands/init.go` that hardcodes the tool list and profile with detection-driven logic:

```go
func RunInit(dir string) error {
	sloopDir := filepath.Join(dir, config.SloopDirName)
	for _, sub := range []string{"context", "skills", "vault", "profiles"} {
		if err := os.MkdirAll(filepath.Join(sloopDir, sub), 0o755); err != nil {
			return err
		}
	}

	// Detect installed known tools; always ensure claude is usable.
	manifests, err := adapter.Load()
	if err != nil {
		return err
	}
	enabled := detect.InstalledKeys(manifests)
	if len(enabled) == 0 {
		enabled = []string{"claude"}
	}
	defaultTool := "claude"
	if !contains(enabled, "claude") {
		defaultTool = enabled[0]
	}

	if err := config.SaveProject(sloopDir, &config.Project{
		Tools:       enabled,
		DefaultTool: defaultTool,
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
	for _, tool := range enabled {
		if err := profile.Save(filepath.Join(sloopDir, "profiles", tool+".yaml"),
			profile.Default(tool)); err != nil {
			return err
		}
	}

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

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
```

Add the imports `"github.com/stroops/sloop/internal/adapter"` and `"github.com/stroops/sloop/internal/detect"` to `init.go`.

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/cli/commands/ -run TestRunInit -v && go build ./...`
Expected: PASS — both `TestRunInitScaffolds` and `TestRunInitFallsBackToClaude`.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/commands/
git commit -m "feat: Drive init tool enablement from detection"
```

---

### Task 10: `run`/`sync` gain `-w` flag, tmux launch, and smart init fallback

**Files:**
- Modify: `internal/cli/commands/run.go`
- Modify: `internal/cli/commands/sync.go`
- Test: `internal/cli/commands/runwflag_test.go`

**Interfaces:**
- Consumes: `session.WorkspaceByName`, `runner.TmuxAvailable`/`TmuxRunner`/`TmuxSessionName`, `Interaction` (Task 5), `RunInit`.
- Produces:
  - `func resolveStartDir(cwd, workspaceFlag string) (string, error)` — if `workspaceFlag` is set, look it up in the registry and return its path; else return `cwd`. Shared by run and sync.
  - Updated `RunRun(startDir, target string, r runner.Runner) error` — unchanged signature; the command layer now (a) resolves `-w`, (b) selects `TmuxRunner` when `runner.TmuxAvailable()`, else `ExecRunner`.
  - run command gains `-w/--workspace` string flag; sync command gains `-w/--workspace` string flag.

- [ ] **Step 1: Write the failing test for `-w` resolution and tmux selection**

`internal/cli/commands/runwflag_test.go`:
```go
package commands

import (
	"path/filepath"
	"testing"

	"github.com/stroops/sloop/internal/runner"
)

func TestResolveStartDirFromRegistry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := RunInit(dir); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	// init registered the workspace under base(dir).
	got, err := resolveStartDir("/somewhere/else", filepath.Base(dir))
	if err != nil {
		t.Fatalf("resolveStartDir: %v", err)
	}
	wantAbs, _ := filepath.Abs(dir)
	gotAbs, _ := filepath.Abs(got)
	if gotAbs != wantAbs {
		t.Fatalf("want %s, got %s", wantAbs, gotAbs)
	}
}

func TestResolveStartDirNoFlagReturnsCwd(t *testing.T) {
	got, err := resolveStartDir("/here", "")
	if err != nil || got != "/here" {
		t.Fatalf("want /here, got %s err=%v", got, err)
	}
}

func TestSelectRunnerPrefersExecWhenNoTmux(t *testing.T) {
	// selectRunner returns a TmuxRunner only when tmux is available; otherwise ExecRunner.
	r := selectRunner("backend", "claude")
	if runner.TmuxAvailable() {
		if _, ok := r.(runner.TmuxRunner); !ok {
			t.Fatalf("expected TmuxRunner when tmux present")
		}
	} else {
		if _, ok := r.(runner.ExecRunner); !ok {
			t.Fatalf("expected ExecRunner when tmux absent")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands/ -run 'TestResolveStartDir|TestSelectRunner' -v`
Expected: FAIL (undefined: resolveStartDir / selectRunner).

- [ ] **Step 3: Add resolver + runner selector and wire flags**

Add to `internal/cli/commands/run.go`:
```go
import (
	// add to existing imports:
	"github.com/stroops/sloop/internal/session"
)

// resolveStartDir maps an optional -w workspace name to a directory via the
// registry; with no flag it returns cwd unchanged.
func resolveStartDir(cwd, workspaceFlag string) (string, error) {
	if workspaceFlag == "" {
		return cwd, nil
	}
	dbPath, err := config.GlobalDBPath()
	if err != nil {
		return "", err
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return "", err
	}
	defer store.Close()
	ws, err := store.WorkspaceByName(workspaceFlag)
	if err != nil {
		return "", fmt.Errorf("workspace %q not found in registry: %w", workspaceFlag, err)
	}
	return ws.Path, nil
}

// selectRunner returns a tmux-backed runner when tmux is available, else exec.
func selectRunner(workspace, tool string) runner.Runner {
	if runner.TmuxAvailable() {
		return runner.TmuxRunner{Session: runner.TmuxSessionName(workspace, tool)}
	}
	return runner.ExecRunner{}
}
```

Update the `runCmd` `RunE` to use the flag and selected runner:
```go
var runWorkspace string

var runCmd = &cobra.Command{
	Use:   "run [tool|profile]",
	Short: "Sync context and launch an AI tool in the workspace",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		startDir, err := resolveStartDir(cwd, runWorkspace)
		if err != nil {
			return err
		}
		target := ""
		if len(args) == 1 {
			target = args[0]
		}
		ws, err := workspace.Resolve(startDir)
		if err != nil {
			return err
		}
		proj, err := config.LoadProject(ws.SloopDir())
		if err != nil {
			return err
		}
		if target == "" {
			target = proj.DefaultTool
		}
		return RunRun(startDir, target, selectRunner(ws.Name, target))
	},
}

func RegisterRun(cmd *cobra.Command) {
	runCmd.Flags().StringVarP(&runWorkspace, "workspace", "w", "", "target a registered workspace by name")
	cmd.AddCommand(runCmd)
}
```

> This requires importing `"github.com/stroops/sloop/internal/workspace"` in run.go (RunRun already uses it indirectly; add the import). The `target` passed to `selectRunner` may be a profile name; using it for the tmux session name is acceptable (session names just need to be stable + unique per tool/profile).

Add the same `-w` flag to sync in `internal/cli/commands/sync.go`:
```go
var syncWorkspace string

// in syncCmd.RunE, replace `cwd` resolution:
//   startDir, err := resolveStartDir(cwd, syncWorkspace)
//   ... then RunSync(startDir, target)

func RegisterSync(cmd *cobra.Command) {
	syncCmd.Flags().StringVarP(&syncWorkspace, "workspace", "w", "", "target a registered workspace by name")
	cmd.AddCommand(syncCmd)
}
```

Update `syncCmd.RunE` to call `resolveStartDir(cwd, syncWorkspace)` before `RunSync`.

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/cli/commands/ -run 'TestResolveStartDir|TestSelectRunner' -v && go build ./...`
Expected: PASS, build OK.

- [ ] **Step 5: Full suite + e2e smoke**

Run:
```bash
go test ./...
go build -o /tmp/sloop ./cmd/sloop
WORK="$(mktemp -d)" && cd "$WORK" && /tmp/sloop init && /tmp/sloop tools && /tmp/sloop doctor && /tmp/sloop status && /tmp/sloop ls
```
Expected: all tests pass; `tools`/`doctor` list claude+cursor; `status` prints a line with `sync:`; `ls` lists the workspace.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/commands/
git commit -m "feat: Add workspace flag and tmux launch to run and sync"
```

---

## Self-Review

**Spec coverage (Plan 2 portion):**
- §8 tmux runner + `attach` → Task 4 ✓ (optional, exec fallback preserved)
- §9.1 detection (tools + tmux, on-demand, no cache) → Task 2 ✓
- §9.2 init auto-enable from detection → Task 9 ✓
- §10 interaction mode (`ask`/`auto`, `--auto`/`-y`, `--no-input`) → Task 1 (config) + Task 5 (helper/flags) ✓
- §11 `sloop status` + mtime freshness → Task 7 ✓
- §12 commands `ls`/`attach`/`tools`/`doctor`/`skill` → Tasks 4, 6, 7, 8 ✓
- §6 Cursor built-in + user adapters (`~/.sloop/adapters/`) → Task 3 ✓
- §3 `-w` workspace flag → Task 10 ✓

**Deferred (correctly absent):** Aider adapter; `install_hint`/any auto-install; deep statusline integration (tmux bar / PS1 / Claude `statusLine`). The interaction mode's `auto`-driven init-on-the-fly during `run` (prompt to init when no `.sloop/`) is intentionally minimal here — `run`'s `-w` path and default-tool selection are covered; the init-on-missing-workspace prompt can be layered using the `Interaction.Confirm` helper from Task 5 if desired, but is not required for Plan 2's commands to function.

**Placeholder scan:** The freshness implementation in Task 7 Step 3 uses `timeType` as a deliberate flag with an explicit instruction to substitute `time.Time` and import `time` — this is called out, not a silent placeholder. No TBD/TODO elsewhere.

**Type consistency:** `detect.Tools(map[string]adapter.Manifest) []ToolStatus`, `detect.InstalledKeys(...) []string`, `adapter.Load() (map[string]Manifest, error)`, `runner.TmuxSessionName/BuildTmuxNewArgs/BuildTmuxAttachArgs`, `sync.Stale(root, sloopDir, adapter.Manifest, profile.Profile)`, `ResolveInteraction(...) Interaction`, `resolveStartDir(cwd, flag)`, `selectRunner(workspace, tool) runner.Runner` are used identically across the tasks that consume them. `config.Project` gains `Mode` without breaking Plan 1 callers (omitempty).
