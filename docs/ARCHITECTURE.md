# Sloop — Architecture Overview

Sloop is a **single, local-first CLI binary**. There is no daemon, no gRPC, no background
service. Every command runs to completion, reading and writing plain files plus a local
SQLite database.

```text
                Developer
                    │
                    ▼
              ┌───────────┐
              │  sloop    │   single binary (Cobra)
              └─────┬─────┘
                    │
   ┌────────────────┼─────────────────────────┐
   ▼                ▼                          ▼
 workspace        sync                       runner
 (.sloop/ +      (canonical context ──▶      (cd to workspace root,
  global          native files:              launch tool; tmux-aware
  registry)       CLAUDE.md, .cursor/...)     with plain-exec fallback)
   │                │                          │
   ▼                ▼                          ▼
 session         adapter                     detect
 (SQLite:        (YAML manifests:            (on-demand: which tools
  workspaces      tool → native file +        + tmux are installed)
  + history)      launch command)


        AI CLIs (Claude Code, Cursor CLI, Aider, …)
                         ▲
                         │  Sloop launches them with context already in place
                         │
        Sloop = local DX layer: sync + launch + profiles + memory
```

## How a launch works

`sloop run <tool|profile>`:

1. **Resolve workspace** — walk up from cwd to find `.sloop/`, or `-w <name>` looks it up in
   the global registry.
2. **Resolve target** — a tool, or a profile (tool + selected skills/context).
3. **Assemble canonical context** — `context/` + the profile's `skills/` + `vault/`.
4. **Render & write native files** — via the tool's YAML adapter manifest (e.g. `CLAUDE.md`).
5. **Record the session** — best-effort row in SQLite.
6. **Launch** — `tmux new-session -c <root>` if tmux is present; otherwise plain
   `exec` in the current terminal.

## Storage

| Location | Contents | Git |
|---|---|---|
| `<project>/.sloop/` | `config.yaml`, `context/`, `skills/`, `vault/`, `profiles/` | committed |
| generated `CLAUDE.md`, `.cursor/rules`, `AGENTS.md` | rendered output | gitignored |
| `~/.sloop/sloop.db` | workspaces registry + session history | machine-local |
| `~/.sloop/adapters/*.yaml` | user-added tool manifests | machine-local |

## Principles

- **Lightweight & local-first** — no daemon, no cloud, pure-Go SQLite (`modernc.org/sqlite`).
- **Build on existing tools** — Sloop syncs context and launches them; it never proxies an
  LLM or replaces a CLI.
- **tmux is optional** — an enhancement where present, never a hard dependency.
- **Declarative adapters** — adding a tool is adding a YAML manifest, not Go code.

See `docs/superpowers/specs/2026-06-26-sloop-mvp-design.md` for the full MVP design.
