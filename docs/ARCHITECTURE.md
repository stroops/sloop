# Sloop — Architecture

Sloop is a **single, local-first CLI binary** (Cobra). No daemon, no gRPC, no background service —
every command runs to completion over plain files plus a local SQLite database. The design goal is
that **all per-provider knowledge lives in declarative adapter manifests** and the rest is small,
composable packages behind clear seams.

```text
                         Developer
                             │
                             ▼
                      ┌──────────────┐
                      │   sloop      │  one binary (cli/commands)
                      └──────┬───────┘
        ┌───────────┬────────┼───────────┬──────────────┐
        ▼           ▼        ▼           ▼              ▼
     adapter      sync     runner       tmux         session
   (YAML manifest: (deliver  (Runner    (multiplexer:  (SQLite: workspaces
    detect/launch/  pointers + interface: tmux/psmux —  + history; WAL +
    context/skills/ skills     exec or    list/capture/  user_version
    hooks/scaffold) symlinks)  tmux)      send/split/    migrations)
        │           │          │          kill)          │
        ▼           ▼          ▼            │             ▼
     detect       scan      hints          ▼          fleetstate
   (which tools  (codebase  (contextual  (ps/send/    (~/.sloop/state:
    are on PATH)  → AGENTS)  tips, i18n)   attach)     hook status markers)

        AI CLIs (Claude Code, Cursor, Codex, Copilot, Gemini, Antigravity)
                                 ▲
        sloop launches them with context already in place, inside a
        `<workspace>__<tool>` session — which is what makes the fleet view work.
```

## Packages (`internal/`)

| Package | Responsibility |
|---|---|
| `cli/commands` | All Cobra commands; the only place that wires the pieces together. |
| `adapter` | Loads embedded + user **manifests** (the single source of per-provider knowledge). |
| `config` | Local project config (`.sloop/config.yaml`) and global config (`~/.sloop/config.yaml`). |
| `detect` | On-demand "which tools / multiplexer are installed" (PATH lookups). |
| `sync` | Context delivery: `AGENTS.md`, pointer files, skills symlinks — all create-if-missing, with `--repair`. |
| `scan` | Heuristic, offline codebase scan → a pre-filled `AGENTS.md` (no LLM). |
| `runner` | The abstract launch seam: `Runner`/`Spec`/`ExecRunner`. tmux-free. |
| `tmux` | The tmux/psmux backend: build args, parse sessions, capture/send/split/kill, status classification, `tmux.Runner`. `Bin()` picks tmux→psmux. |
| `fleetstate` | Per-session status markers under `~/.sloop/state`, written by tool hooks, read by `ps`. |
| `session` | SQLite store (workspaces registry + session history); WAL + `PRAGMA user_version` migrations. |
| `hints` | Embedded, throttled, i18n (en/vi) education tips shown after commands. |
| `tui` | Zero-dependency raw-mode menu (`SelectMenu`/`SelectAction`) + color helpers (NO_COLOR-aware). |
| `workspace` | Resolve the workspace (walk up to `.sloop/`, or `-w` via the registry). |

## How a launch works

`sloop run [tool]`:

1. **Resolve workspace** — walk up from cwd to `.sloop/`, or `-w <name>` via the global registry.
2. **Resolve tool** — the argument, or `default_tool` from `.sloop/config.yaml` (no per-tool profile
   files; the enabled set is the single source).
3. **Sync** — ensure `AGENTS.md`; write the tool's pointer file if it's a pointer-mode adapter and the
   file is absent; symlink `.sloop/skills` into the tool's skills dir.
4. **Record the session** — best-effort row in SQLite (a failure warns, never blocks the launch).
5. **Launch** — `tmux.Runner` (`<workspace>__<tool>` session) when a multiplexer is present, else
   `runner.ExecRunner` (plain exec). Args after `--` pass straight through.

`sloop sync` runs steps 1–3 only. The fleet commands (`ps`, `send`, `attach`, `kill`) operate purely
over the multiplexer + `fleetstate`, never touching the AI provider's process or API.

## Storage

| Location | Contents | Git |
|---|---|---|
| `<project>/AGENTS.md` | canonical context (hand-authored) | committed |
| `<project>/.sloop/` | `config.yaml`, `skills/` (committed); `vault/` (gitignored) | committed |
| generated `CLAUDE.md`, `.claude/skills`, … | pointer files + symlinks | your choice |
| `~/.sloop/sloop.db` | workspaces registry + session history (SQLite/WAL) | machine-local |
| `~/.sloop/state/*.json` | per-session hook status markers (TTL'd) | machine-local |
| `~/.sloop/adapters/*.yaml` | user-added / override tool manifests | machine-local |
| `~/.sloop/config.yaml`, `hints-state.json` | machine prefs + hint throttle state | machine-local |

## Principles & seams

- **Manifest-driven** — every provider-aware feature reads `adapter` manifests; never hardcode a tool.
  See [ADAPTERS.md](ADAPTERS.md).
- **Provider-respecting** — status uses each tool's *own* hooks or non-invasive `capture-pane` of your
  own terminal; sloop never intercepts the provider's API or process.
- **Lightweight & local-first** — one CGO-free binary (`modernc.org/sqlite`), no daemon, no cloud.
- **The multiplexer is a seam** — tmux today, psmux on Windows, a future backend behind the same
  `runner.Runner`/`tmux` boundary.
- **Cede in-repo orchestration** — worktrees/dashboards/conflict-detection belong to ntm / Claude
  Squad; sloop's edge is portable context + the cross-repo fleet.
