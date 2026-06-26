# Sloop Sync v2 — Hardening / Gap-Fill — Design Spec

**Date:** 2026-06-26
**Status:** Approved (design), pending implementation plan
**Builds on:** `2026-06-26-sloop-sync-v2-design.md` (Model B). This spec does **not** change the
delivery model; it closes the gaps between that spec and the shipped implementation, and hardens
the symlink delivery against real-world breakage.
**Reconciles:** `docs/plans/sloop-backlog.md` Phase 3 — see §10.

---

## 1. Why

Sync v2 (Model B) is implemented and green: `AGENTS.md` canonical, pointer files
create-if-missing, `.sloop/skills/` symlinked into each tool's skills dir. While reviewing the
result against the v2 design spec and the backlog, three real gaps remain:

1. **`sloop sync --all` is specified but not built.** The v2 design (§5) lists
   `sloop sync [tool | --all]`, but `syncCmd` only accepts a single optional target. A user who
   enabled multiple tools must run `sync` once per tool.
2. **Skills symlinks are fragile under moves and deletions.** The delivered symlink stores an
   **absolute** path to `<root>/.sloop/skills`. Renaming or moving the repository turns every
   skill symlink into a dangling link, which `SkillsState` then misreports. There is also no
   distinct "broken" state — a dangling-but-correct symlink currently reads as `linked`.
3. **No recovery path for a foreign/occupied target.** Model B (correctly) never overwrites a
   file it didn't create, so a `foreign` pointer or an occupied skills dir is reported and then
   left forever. There is no opt-in, non-destructive way to say "I want sloop's delivery here —
   move mine aside."

This spec is explicitly the **opposite** of the backlog's old "Two-Way Sync Engine
(pull/diff/undo)": that engine existed to reconcile full-copy duplicates under v1 and is **not
needed under Model B** (the v2 spec §10 already says so). What is needed is small, surgical
hardening of the one-way delivery plus the missing `--all`.

---

## 2. Scope

**In scope (this spec):**
- `sloop sync --all` — deliver to every enabled tool in one invocation.
- **Relative** skills symlinks (survive workspace moves) + a true `broken` state + self-heal of a
  sloop-owned dangling symlink during normal `sync`.
- `sloop sync --repair` — opt-in, **non-destructive** recovery: back the foreign occupant aside
  (rename to `<name>.sloopbak-<timestamp>`), then create sloop's artifact. Never deletes.
- Reconcile the `status` skills vocabulary to states that are actually detectable without markers.

**Explicitly out of scope (and not coming back under Model B):**
- `sloop sync pull` — there is nothing to ingest: skills are the *same* files via symlink, and
  `AGENTS.md` is canonical/hand-authored. (Copy-fallback workspaces are the one theoretical
  exception; see §9.)
- `sloop sync diff` as a separate command — `status` already reports per-channel state; `--repair`
  is the action. No standalone diff/validate engine.
- `sloop sync undo` / `.cache/history` — v2 delivery only ever **creates**; `--repair` only ever
  **renames aside** (the backup *is* the undo). No history log is built.
- Managed-region markers — rejected by the v2 spec and not reconsidered here.

---

## 3. Gap 1 — `sloop sync --all`

`sloop sync` gains a boolean `--all` flag, mutually exclusive with a positional `tool|profile`
argument.

- `sloop sync` → default tool (unchanged).
- `sloop sync <tool|profile>` → that target (unchanged).
- `sloop sync --all` → iterate **`config.Project.Tools`** (the enabled tools), delivering to each.

Behavior of `--all`:
- `AGENTS.md` is ensured once (the first tool creates it; the rest see it and skip — `EnsureAgents`
  is already idempotent).
- For each tool, run the same context + skills delivery as single-tool sync.
- Output prefixes each action line with the tool key, e.g. `claude: created CLAUDE.md`,
  `gemini: GEMINI.md exists, left as-is`, `cursor: native (AGENTS.md)`.
- Unknown tool in the enabled list → warn for that tool, continue the others (don't abort the
  whole `--all` run for one bad adapter).
- `--all` together with a positional arg → usage error.

Internally, the per-tool delivery block in `RunSync` is extracted into a shared helper so single
and `--all` paths can't drift:

```go
// syncOne delivers AGENTS.md + context + skills for one resolved manifest,
// returning human-readable action lines (caller prefixes with tool key for --all).
func syncOne(root, sloopDir string, m adapter.Manifest) ([]string, error)
```

`RunSyncAll(startDir string) ([]string, error)` resolves the workspace + project, loads manifests
once, then calls `syncOne` for each `proj.Tools` entry, prefixing lines.

---

## 4. Gap 2 — Robust skills symlink (relative + broken state + self-heal)

### 4.1 Relative symlinks

Skills are linked with a **relative** target computed from the link's own directory, not an
absolute path:

```
<root>/.claude/skills  ->  ../.sloop/skills
```

(`reltarget = filepath.Rel(filepath.Dir(target), source)`, e.g. from `<root>/.claude` to
`<root>/.sloop/skills` ⇒ `../.sloop/skills`.)

This survives renaming or moving the repository, which an absolute link does not. A single helper
produces the relative form for both delivery and state so they cannot disagree:

```go
func skillsLinkTarget(root, sloopDir, manifestTarget string) (linkPath, relSource string)
```

### 4.2 `broken` state and self-heal

A symlink is **broken** when it points at our skills dir (its readlink equals `relSource`) but the
destination does not resolve (`os.Stat(linkPath)` fails). Two sub-cases:

| Cause | Detection | Action |
|---|---|---|
| `.sloop/skills` source genuinely missing | readlink == relSource **and** source dir absent | report `broken`; **do not** silently relink (the workspace itself is damaged — fixing `.sloop/skills` is the user's call). `sync` prints `skills source .sloop/skills missing`. |
| Stale **absolute** link from a pre-hardening sync, now dangling after a move | readlink is an absolute path **and** unresolvable | sloop recognizes its own old absolute form (basename `skills`, parent `.sloop`) and **re-links relatively** during normal `sync` (self-heal, no flag). Prints `relinked .claude/skills`. |

The first case is reported, not auto-fixed, because removing/recreating can't conjure a missing
source. The second is a safe, sloop-owned upgrade and is healed automatically.

### 4.3 Reconciled skills states (§6 of the v2 spec)

`SkillsState(root, sloopDir, m) string` returns one of:

| state | meaning |
|---|---|
| `none` | adapter declares no `skills.target` |
| `missing` | target path does not exist |
| `linked` | symlink → our skills dir **and** it resolves |
| `broken` | symlink → our skills dir but destination unresolvable |
| `present` | a real dir/file (or a non-sloop symlink) occupies the target |

This **supersedes** the v2 spec §5 `skills:<linked|copied|none|foreign>` wording. `copied` is
dropped: under Model B's no-markers rule, a copy-fallback directory is byte-content indistinguishable
from a user-authored dir, so both honestly read as `present`. `foreign` is renamed `present` to
reflect that sloop makes no ownership claim about a real directory it finds. The `status` line
becomes:

```
⚓ <ws> · <tool> · agents:<ok|missing> · ctx:<ok|missing|foreign> · skills:<none|missing|linked|broken|present>
```

---

## 5. Gap 3 — `sloop sync --repair` (non-destructive)

`sloop sync` gains a boolean `--repair` flag. Default sync is unchanged (create-if-missing, warn on
occupied). With `--repair`, when sloop *would* otherwise leave a target alone because something
non-sloop occupies it, it instead:

1. Renames the occupant aside to `<name>.sloopbak-<RFC3339-ish timestamp>` (kept, never deleted).
2. Creates sloop's correct artifact in its place.

Applies to:
- **Pointer context** in `foreign` state → back up the foreign `CLAUDE.md`/`GEMINI.md`, write the
  pointer. Prints `repaired CLAUDE.md (backup: CLAUDE.md.sloopbak-…)`.
- **Skills** in `present` state → back up the occupying dir/file, create the relative symlink.
- **Skills** in `broken` state (absolute legacy link) → self-heal already covers this without
  `--repair`; with `--repair` it's also fine (idempotent).

Never applies to:
- **`AGENTS.md`** — canonical and hand-authored; `--repair` must never rename or replace it.
- Anything already in `ok`/`linked`/`missing` state (nothing to repair).

`--repair` composes with `--all` (`sync --all --repair` repairs every enabled tool). The backup is
the undo: a user restores by deleting sloop's artifact and renaming the `.sloopbak-…` back.

New delivery functions (kept separate so existing create-if-missing callers are untouched):

```go
func RepairContext(root string, m adapter.Manifest) (Action, error) // ActionRepaired | ActionSkipped | ...
func RepairSkills(root, sloopDir string, m adapter.Manifest) (Action, error)
```

New `Action` value `ActionRepaired` (and `ActionRelinked` for the self-heal) for command output.

---

## 6. Adapter manifest & config

**Unchanged.** No new YAML fields. `--all` reads the existing `config.Project.Tools`; relative
symlinks and repair are pure delivery-layer behavior. No `go mod` changes.

---

## 7. Commands summary (delta only)

| Command | Change |
|---|---|
| `sloop sync` | + `--all` (all enabled tools), + `--repair` (non-destructive backup-then-create). Single/positional forms unchanged. |
| `sloop run` | Unchanged. Still single-tool; still calls `RunSync` for that tool. (`run` does not gain `--all` — you launch one tool.) |
| `sloop status` | Reconciled skills vocabulary (`none|missing|linked|broken|present`). No flag change. |
| `sloop init` | Unchanged. |

---

## 8. Error handling

- `--all` with a positional arg → usage error before any I/O.
- `--all`: a single unknown/unloadable tool warns and is skipped; the run continues and exits 0 if
  the rest succeed.
- `--repair` backup rename fails (permissions) → error for that artifact, continue other tools;
  sloop never proceeds to create over a failed-to-move occupant (no data loss).
- Broken symlink with genuinely missing `.sloop/skills` source → reported, not auto-fixed.
- All existing v2 error rules (unwritable AGENTS.md, symlink→copy fallback) stay.

---

## 9. The one theoretical two-way case: copy-fallback

When skills were **copied** (symlink unavailable, e.g. Windows without privilege), edits a tool
makes inside its copy do **not** flow back to `.sloop/skills`. This is the single residual
divergence Model B can produce. This spec deliberately does **not** build a `sync pull` for it
because:

- It only occurs on symlink-incapable setups (rare; modern Windows 10+ developer mode and macOS/Linux
  all symlink fine).
- A pull would re-introduce exactly the copy-reconciliation complexity Model B removed.

It is recorded here as a known limitation. If it ever becomes real, the minimal fix is a
copy-fallback-only `sync pull` (mirror tool-copy → `.sloop/skills` with conflict detection), scoped
to that case alone — not a general two-way engine.

---

## 10. Backlog reconciliation

`docs/plans/sloop-backlog.md` Phase 3 is updated by this spec:

- **3.1 Symlink Strategy (Zero-copy Sync)** → **done** by sync v2.
- **3.2 Two-Way Sync Engine (Pull, Push, Diff, Undo)** → **superseded** by Model B. The pull/diff/undo
  engine is not built; its goals are met structurally (symlink = two-way; `AGENTS.md` canonical) or
  replaced by the smaller, safe `--repair` (§5). The backlog entry is annotated accordingly.

---

## 11. Testing (TDD)

- **`--all`:** enabled = `{claude, cursor}` → one run delivers `CLAUDE.md` pointer + `.claude/skills`
  link for claude and `native` no-op for cursor; output prefixed per tool; `AGENTS.md` created once.
- **`--all` + positional arg** → usage error.
- **`--all` with an unknown enabled tool** → warns for it, still delivers the others, exits 0.
- **Relative symlink:** `os.Readlink(.claude/skills) == "../.sloop/skills"`; resolves to the same
  inode as `.sloop/skills`.
- **Move survival:** create link, rename workspace root, re-run `SkillsState` → still `linked`
  (relative link resolves).
- **`broken` state:** legacy absolute link with missing destination → `SkillsState == "broken"`;
  `sync` self-heals the legacy-absolute case to a relative link (`relinked`); genuinely-missing
  source is reported, not relinked.
- **`--repair` context:** foreign `CLAUDE.md` → backed up to `CLAUDE.md.sloopbak-…` (content
  preserved) + pointer written; `AGENTS.md` is never touched by repair.
- **`--repair` skills:** a real `.claude/skills` dir → backed up + replaced by the relative symlink;
  original files preserved under the backup name.
- **Non-destructive guarantee:** assert the backup file/dir exists with original bytes after every
  repair (no deletes).
- **status:** prints the reconciled `skills:` vocabulary for `linked`/`broken`/`present`/`missing`/
  `none`.
- **Integration smoke:** `init` → enable two tools → `sync --all` → move repo → `status` still
  `linked` → drop a foreign `CLAUDE.md` → `sync --repair` backs it up and writes the pointer.

---

## 12. Out of scope (unchanged from v2 §10)

- General two-way pull/diff/undo engine (this spec replaces the need).
- Vault delivery / Obsidian plugins (Phase 6).
- Windows multiplexer mapping (Phase 7).
- Per-profile skill selection / richer launch presets.
