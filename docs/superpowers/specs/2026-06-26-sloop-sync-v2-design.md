# Sloop Sync v2 — Design Spec

**Date:** 2026-06-26
**Status:** Approved (design), pending implementation plan
**Supersedes:** the context-delivery model in `2026-06-26-sloop-mvp-design.md` §5–6 (one-way full-copy passthrough from `.sloop/context/`).

---

## 1. Why

Sync v1 treats `.sloop/context/` as the source of truth and **overwrites** each tool's
native file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`) with an identical full copy. Two problems:

1. **Clobber:** developers hand-author `AGENTS.md` (rich, canonical) and keep `CLAUDE.md` as a
   thin pointer to it. v1's `sync` would destroy both, replacing them with a dump of
   `.sloop/context/`. Data loss.
2. **Wrong source of truth & redundancy:** context belongs to the working repository, so the
   canonical file should be the repo's `AGENTS.md` — not a parallel store in `.sloop/`. Copying
   the same content into 5 files is redundant.

Sync v2 adopts **Model B**: `AGENTS.md` is the canonical context; sloop is a thin **CLI
wrapper** that only fills in what's missing and never destroys hand-written files.

---

## 2. Canonical sources

| Channel | Canonical (developer-owned) | Committed to git |
|---|---|---|
| Context (prose) | **`AGENTS.md`** at the repo/workspace root, hand-written | ✅ |
| Skills (reusable capabilities) | **`.sloop/skills/*.md`**, project-local | ✅ |
| Vault (2nd brain) | `.sloop/vault/*.md` — **not** auto-delivered to tools | ✅ |

**`.sloop/context/` is removed.** `AGENTS.md` replaces it. Vault stays for `sloop show`/RAG
(future), not part of sync.

---

## 3. Delivery model: "fill in what's missing, never overwrite"

sloop is a wrapper: it **creates missing artifacts and stops there**. It never rewrites a file
it didn't just create, and it uses **no markers / managed regions** inside the user's files.

### 3.1 Context delivery (per tool)

Two modes, declared by the adapter:

- **`native`** — the tool reads `AGENTS.md` directly (cursor, codex, copilot). sloop generates
  **no** context file; it only ensures `AGENTS.md` exists.
- **`pointer`** — the tool reads its own file (claude→`CLAUDE.md`, gemini→`GEMINI.md`). sloop
  generates a thin pointer file that redirects to `AGENTS.md`.

**Pointer generation is create-if-missing and idempotent:**

| State of the pointer file | sloop action |
|---|---|
| missing | create it with the pointer content |
| exists, byte-identical to what sloop would write | skip silently (idempotent) |
| exists, different content | **leave it untouched**, print a warning (`CLAUDE.md exists, left as-is`) |

There is **no `--force`** in v2. To regenerate, the developer deletes the file and re-syncs.

**Pointer content** (generalized from the developer's preferred format; `<Tool>` is the
adapter `name`, `<file>` the pointer filename):

```markdown
# <file>

This file provides guidance to <Tool> when working with code in this repository.

**Note**: This project uses AGENTS.md for detailed guidance.

## Primary Reference

See `AGENTS.md` in this same directory for the main project documentation and guidance.
```

### 3.2 Skills delivery (per tool)

If the adapter declares a `skills.target`, sloop links the canonical skills directory into the
tool's native skills directory so the tool auto-loads them:

- **Primary:** create a symlink **at** `<root>/<skills.target>` **pointing to** `.sloop/skills/`
  (zero-copy; editing either side edits the same files — two-way for free).
- **Fallback:** when symlinking is unavailable (Windows without privilege, or the target's
  parent is on a filesystem that rejects symlinks), **copy** the directory contents instead.

**Create-if-missing semantics (same philosophy as pointers):**

| State of the target path | sloop action |
|---|---|
| missing | create the symlink (or copy) |
| already the correct symlink → `.sloop/skills/` | skip silently |
| a real directory/file exists there (not our symlink) | **leave it**, print a warning |

Adapters without a `skills.target` deliver no skills (nothing to link).

### 3.3 Vault

Not delivered to tools by sync. Reserved for `sloop show`/RAG (future phases).

---

## 4. Adapter manifest v2

The manifest replaces the v1 `outputs:` list with two declarative channels:

```yaml
name: Claude Code
detect: claude
launch: claude
context:
  mode: pointer        # "pointer" (generate file) | "native" (reads AGENTS.md)
  file: CLAUDE.md      # required when mode: pointer; omitted for native
skills:
  target: .claude/skills   # dir (relative to workspace root) to link .sloop/skills into; omit if none
```

### 4.1 Built-in manifests (v2)

| key | detect/launch | context.mode | context.file | skills.target |
|---|---|---|---|---|
| claude | `claude` | pointer | `CLAUDE.md` | `.claude/skills` |
| gemini | `gemini` | pointer | `GEMINI.md` | _(none, best-guess unknown)_ |
| cursor | `agent` | native | — | _(none)_ |
| codex | `codex` | native | — | _(none)_ |
| copilot | `copilot` | native | — | _(none)_ |

`skills.target` values are **best-guess and editable** in the YAML; only `claude`'s
`.claude/skills` is set initially. Others are left empty until confirmed (the user can add a
target to any manifest, including user adapters in `~/.sloop/adapters/`). The mechanism, not the
exact paths, is what this spec fixes.

---

## 5. Commands

### `sloop init`
- Create a starter **`AGENTS.md`** at the workspace root **if it does not already exist**
  (never overwrite an existing one).
- Create `.sloop/{skills,vault,profiles}` and `config.yaml`. **No `.sloop/context/`.**
- Enable detected tools + write one profile per tool (see §6). Register the workspace.

Starter `AGENTS.md`:
```markdown
# AGENTS.md

Project guidance for AI coding tools. Describe the project, conventions, and constraints here.
This is the canonical context; sloop points other tools (CLAUDE.md, GEMINI.md, …) at this file.
```

### `sloop sync [tool | --all]`
For each targeted enabled tool:
1. Ensure `AGENTS.md` exists (create the starter if missing — same as init).
2. Context: if `mode: pointer`, apply the create-if-missing pointer rules (§3.1); if `native`,
   nothing beyond step 1.
3. Skills: if `skills.target` set, apply the symlink/copy create-if-missing rules (§3.2).

`sync` prints one line per action (`created CLAUDE.md`, `linked .claude/skills`,
`CLAUDE.md exists, left as-is`).

### `sloop run [tool|profile] [-w <ws>]`
Unchanged flow: resolve workspace → sync (as above) → launch (tmux-aware). Only the sync step's
internals change.

### `sloop status`
Redefined — `mtime` freshness is gone (there is no generated full-copy to age). Status reports
delivery state for the default tool:
```
⚓ <workspace> · <tool> · agents:<ok|missing> · ctx:<ok|missing|foreign> · skills:<linked|copied|none|foreign>
```
- `ctx`: `ok` (pointer present or native), `missing` (pointer mode, file absent), `foreign`
  (file exists but isn't sloop's pointer).
- `skills`: `linked`/`copied` (delivered), `none` (no target declared), `foreign` (a non-sloop
  dir occupies the target).

---

## 6. Profile simplification

In Model B a profile no longer selects context/skills/vault subsets (context = the single
`AGENTS.md`; skills = the whole symlinked directory). A profile collapses to a named tool
binding:

```yaml
tool: claude
```

- `profile.Profile` keeps only `Tool string` (drop `Context`, `Skills`, `Vault`).
- `profile.Default(tool)` → `{Tool: tool}`.
- `resolveProfile(...)` still maps a `run`/`sync` target (tool name or profile name) to a tool.

(Per-profile skill selection and richer launch presets — e.g. extra launch args — are a
possible future addition, out of scope here.)

---

## 7. Code migration (within this change)

This is a **breaking change**; the project is pre-release, so there is **no automated
migration** — existing v1 workspaces are recreated with `sloop init`. The spec author accepts
re-init; no upgrade command is built.

Affected packages:

- **`internal/adapter`**: replace `Output`/`Outputs` + `Render` with the v2 `Context{Mode,File}`
  and `Skills{Target}` structs. Update all five built-in YAML manifests. `LoadBuiltin`/`Load`
  signatures (returning `map[string]Manifest`) are unchanged.
- **`internal/sync`**: remove `Assemble`, `WriteNativeFiles`, and `Stale`/`freshness.go`
  (all `.sloop/context/`- and full-copy-based). Add:
  - `EnsureAgents(root string) (created bool, err error)`
  - `SyncContext(root string, m adapter.Manifest) (Result, error)` (pointer create-if-missing)
  - `SyncSkills(root, sloopDir string, m adapter.Manifest) (Result, error)` (symlink + copy fallback)
  - `pointerContent(toolName, file string) string`
  - where `Result` is a small enum/struct describing the action taken (created/skipped/foreign/linked/copied) for command output and `status`.
- **`internal/cli/commands`**: `init.go` (AGENTS.md starter, drop `context/`), `sync.go`
  (call the new sync functions), `run.go` (uses the new sync via `RunSync`), `status.go`
  (new delivery-state line), and `resolveProfile` updated for the simplified `Profile`.
- **`internal/config`**: unchanged (Project still lists tools + default; `mode` stays).

Removed user-visible artifacts: `.sloop/context/`, generated full-copy `CLAUDE.md`/`AGENTS.md`/
`GEMINI.md` (AGENTS.md is now authored; CLAUDE.md/GEMINI.md are thin pointers).

---

## 8. Error handling

- `AGENTS.md` unwritable → error (sync can't proceed for that tool).
- Pointer target exists & differs → warn, continue (never overwrite).
- Skills target occupied by a non-sloop dir → warn, continue (never overwrite).
- Symlink creation fails → fall back to copy; if copy also fails → error for that tool, continue others.
- Unknown tool (no adapter) → clear error.

---

## 9. Testing (TDD)

- **Pointer create-if-missing:** missing→created; identical→skipped; different→left+warned (assert file bytes unchanged).
- **Native mode:** no context file generated; `AGENTS.md` ensured.
- **Skills symlink:** target created as symlink to `.sloop/skills`; re-sync idempotent; foreign dir left untouched + warned.
- **Copy fallback:** force the copy path (e.g. an injected `symlink func` returning an error) and assert files copied.
- **init:** creates `AGENTS.md` starter (and does not overwrite an existing `AGENTS.md`); creates `.sloop/{skills,vault,profiles}`; no `context/`.
- **status:** reports `agents/ctx/skills` states correctly for pointer and native tools.
- **Integration:** `init` then `sync claude` in a temp dir → `CLAUDE.md` pointer + `.claude/skills` symlink present; running `sync claude` again is a no-op.

---

## 10. Out of scope (future phases)

- `sloop sync pull` / `diff` / `undo` two-way engine (Phase 3.2) — not needed under Model B
  (symlink is already two-way; AGENTS.md is already canonical).
- Managed-region markers (deliberately rejected — too invasive for a CLI wrapper).
- Vault delivery / external vault plugins (Obsidian) (Phase 6).
- Project-shared/committed session metadata for team resume (separate feature).
- Per-profile skill selection and richer launch presets.
- Windows multiplexer mapping (Phase 7) — unrelated to sync.
