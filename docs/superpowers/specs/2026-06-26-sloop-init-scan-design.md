# Sloop `init --scan` — Heuristic Codebase Scan — Design Spec

**Date:** 2026-06-26
**Status:** Approved (design), pending implementation plan
**Phase:** 5 (AI-Driven Workspace Intelligence) — **Step A**, the **non-LLM** precursor.
**Relates to:** `docs/plans/sloop-backlog.md` Phase 5; Model B (`2026-06-26-sloop-sync-v2-design.md`).

---

## 1. Why

The real-world flow is **codebase-first**: a developer has an existing repo, then adopts sloop.
Today `sloop init` writes an **empty** starter `AGENTS.md` (a generic stub via
`sync.EnsureAgents`) — it inspects *which tools are installed* but reads **nothing** about the
project itself. The very first thing a user must do is hand-write the whole `AGENTS.md`.

`sloop init --scan` closes that gap **without any LLM**: a pure-Go heuristic reads the repo's
marker files and structure and writes an `AGENTS.md` that's already populated with detected
facts (language, build/test commands, layout) plus clearly-marked spots to fill in. Deterministic,
free, no API key, always works offline.

This is deliberately the **non-LLM** step. A later step (Phase 5 Step C) can reuse the eventual
minimal LLM client to *enrich* this scaffold into prose — but the heuristic scaffold stands on its
own and ships first.

---

## 2. Scope

**In scope:**
- New pure-Go package `internal/scan`: `Scan(root) Report` + `Report.AgentsMarkdown() string`.
- `sloop init --scan` flag: when set and `AGENTS.md` is absent, write the scanned scaffold instead
  of the empty starter. Tool/profile/registry behavior of `init` is unchanged.
- Detection from **root-level marker files** + a **shallow (one-level) directory listing** — no
  deep tree walk, no file-content parsing beyond a few well-known manifests.

**Out of scope (later / other specs):**
- LLM-enriched `AGENTS.md` (Phase 5 Step C) — reuses the AI-doctor client when it exists.
- AI `sloop doctor` (Phase 5 Step B).
- Making `--scan` the **default** when a codebase is detected (possible later; v1 keeps it an
  explicit opt-in flag).
- Deep static analysis, dependency graphs, monorepo per-package scanning.

---

## 3. Naming

The existing `internal/detect` package means **tool** detection (`detect.InstalledKeys`). To avoid
overloading "detect", the codebase feature is **`scan`**: package `internal/scan`, flag
`sloop init --scan`. (`init` already auto-detects *tools*; `--scan` adds *codebase* awareness.)

---

## 4. Detection signals (v1, deterministic)

All reads are at the repo root (plus a one-level dir listing). Each signal is best-effort: a
missing/garbled file contributes nothing, never an error.

| Signal | Source | Example output |
|---|---|---|
| **Project name** | `go.mod` `module`, `package.json` `name`, `Cargo.toml` `[package] name`, `pyproject.toml` `[project] name`; else dir basename | `github.com/stroops/sloop` → `sloop` |
| **Language(s)** | marker files: `go.mod`→Go; `package.json`→JavaScript (TypeScript if `tsconfig.json`); `Cargo.toml`→Rust; `pyproject.toml`/`setup.py`/`requirements.txt`→Python; `pom.xml`/`build.gradle`→Java/Kotlin; `Gemfile`→Ruby; `composer.json`→PHP | `Go`, `TypeScript` |
| **Language version** | `go.mod` `go 1.x`; `package.json` `engines.node`; `.python-version` | `Go 1.26` |
| **Build / test / lint / run commands** | per-language defaults, overridden by `package.json` `scripts` and `Makefile` targets when present | `go test ./...`; `npm run build` |
| **Project layout** | one-level dir listing filtered to known-meaningful names (`cmd internal pkg src app lib test tests docs api web internal/...`) | `cmd/, internal/, docs/` |
| **Summary seed** | existing `README.md`: first `# H1` title + first non-empty paragraph | "Sloop is a lightweight CLI…" |

**Command inference rules:**
- Go → build `go build ./...`, test `go test ./...`, lint `go vet ./...` (or `golangci-lint run` if `.golangci.yml` exists).
- Node → if `package.json` has `scripts.build`/`scripts.test`/`scripts.lint`, surface `npm run <name>` (or the detected package manager: `pnpm`/`yarn` by lockfile); else omit.
- Rust → `cargo build`, `cargo test`, `cargo clippy`.
- Python → test `pytest` if `pytest`/`tests/` present, else `python -m unittest`; lint `ruff check` if `ruff` configured.
- `Makefile` present → its `build`/`test`/`lint`/`run` targets (if any) win as `make <target>` (the project's own canonical commands).

Unknown/empty results are simply omitted from the output (no empty headings with no content,
except the intentional fill-in placeholders in §5).

---

## 5. Output: the scanned `AGENTS.md`

`Report.AgentsMarkdown()` renders a populated scaffold. Detected facts are stated; everything the
heuristic can't know is an explicit, clearly-marked placeholder the user fills in.

```markdown
# AGENTS.md

Project guidance for AI coding tools. This file is the canonical context; sloop points other
tools (CLAUDE.md, GEMINI.md, …) at it.

## Project

**sloop** — <!-- one-line description; seeded from README below if found -->
> Sloop is a lightweight CLI that improves the developer experience of AI coding tools.

## Tech stack

- Go 1.26 _(detected)_
- TypeScript _(detected)_

## Build, test & lint

```sh
go build ./...
go test ./...
go vet ./...
```

## Project structure

- `cmd/` — entrypoints
- `internal/` — application packages
- `docs/` — documentation

## Conventions

<!-- Add coding standards, architectural rules, and constraints the agent must follow. -->
```

Rules:
- Sections with no detected content are dropped (e.g. no "Tech stack" if nothing matched), **except**
  `## Conventions`, which is always present as the key fill-in prompt.
- Every machine-asserted fact is tagged `_(detected)_` so the user knows what to verify.
- The README summary, if found, is quoted under `## Project` as a seed (not asserted as canonical).
- The result is plain Markdown — no sloop markers, consistent with Model B.

---

## 6. CLI & integration

### `sloop init --scan`
- Add a boolean `--scan` flag to `initCmd`.
- `init` behavior is unchanged except for the `AGENTS.md` body:
  - `--scan` **and** `AGENTS.md` absent → write `scan.Scan(dir).AgentsMarkdown()`.
  - otherwise → the existing empty starter (`sync.EnsureAgents`).
- **Create-if-missing always holds:** `--scan` never overwrites an existing `AGENTS.md` (same Model
  B rule). Scanning an already-initialized repo with a hand-written `AGENTS.md` is a no-op for that
  file.

### sync helper
Generalize the writer so init can supply scanned content:
```go
// EnsureAgentsContent writes content to AGENTS.md if missing; skips if present.
func EnsureAgentsContent(root, content string) (Action, error)
// EnsureAgents keeps its signature, delegating with the default starter.
func EnsureAgents(root string) (Action, error)
```

### `RunInit`
`RunInit(dir string, scan bool) error` — the only signature change; existing callers pass `false`.

---

## 7. Package design (`internal/scan`)

Mirrors `internal/detect`'s style: small structs, plain functions, sorted/deterministic output,
best-effort reads, no external deps.

```go
package scan

type Report struct {
	Name      string   // project name
	Languages []Lang   // detected languages (+ optional version), sorted
	Commands  []Command // build/test/lint/run, in a fixed order
	Layout    []string // notable top-level dirs, sorted
	Summary   string   // README seed (may be "")
}

type Lang struct{ Name, Version string }
type Command struct{ Label, Cmd string } // Label ∈ build|test|lint|run

func Scan(root string) Report
func (r Report) AgentsMarkdown() string
```

`Scan` reads only the root dir entries + a fixed set of manifest files; it never walks the tree or
shells out. Deterministic given the same inputs.

---

## 8. Error handling

- Unreadable/missing marker files → skipped, contribute nothing (never fail the scan).
- `Scan` never returns an error; a totally unrecognized repo yields a `Report` with just `Name`
  (dir basename) + the `## Conventions` placeholder — i.e. it degrades to roughly today's starter.
- `init --scan` write errors surface as normal `init` errors.

---

## 9. Testing (TDD)

- **Go repo:** temp dir with `go.mod` (`module x`, `go 1.26`) + `cmd/`,`internal/` → `Report.Name=="x"`,
  Languages has `Go 1.26`, Commands include `go test ./...`, Layout has `cmd`,`internal`.
- **Node/TS repo:** `package.json` with `name` + `scripts.build`/`scripts.test` + `tsconfig.json` →
  TypeScript, `npm run build`/`npm test` (or pnpm/yarn by lockfile).
- **Makefile precedence:** a `Makefile` with `test:` target → `make test` wins over the language default.
- **Empty/unknown dir:** `Scan` returns `Name`=basename, no languages; `AgentsMarkdown` still has
  `# AGENTS.md` + `## Conventions`.
- **README seed:** a `README.md` with an H1 + paragraph → quoted under `## Project`.
- **`init --scan` integration:** in a temp Go repo, `RunInit(dir, true)` writes an `AGENTS.md`
  containing `go test ./...`; `RunInit(dir, false)` writes the plain starter; `--scan` over an
  existing `AGENTS.md` leaves it untouched.

---

## 10. Out of scope (future)

- LLM enrichment of the scaffold (Phase 5 Step C).
- AI `sloop doctor` (Phase 5 Step B).
- `--scan` as the default; monorepo/per-package scanning; deep analysis.
