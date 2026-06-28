# Changelog

All notable changes to Sloop are documented here. This project adheres to
[Semantic Versioning](https://semver.org/).

## v0.1.0 — first release

The initial public release: the local-first control layer for your AI coding CLIs.

### Portable context
- `AGENTS.md` as the single canonical context; create-if-missing pointer files
  (`CLAUDE.md`, `GEMINI.md`, …) — never overwrites your files.
- `.sloop/skills` symlinked into each tool's skills dir (self-healing, copy fallback).
- `sloop sync` (`--all`, `--repair`) and `sloop status`.
- `sloop init` scaffolds the workspace and delivers context immediately; `--scan`
  pre-fills `AGENTS.md` from the codebase (offline, no LLM); `--scaffold` creates each
  tool's standard folders.

### Launch
- `sloop run` syncs context, then launches in a managed session. The target can be a
  tool (`claude`), its binary (`agent` → cursor), or a model alias (`opus` → its vendor's
  home CLI). Flags: `-m/--model`, `-e/--effort` (low|medium|high), `-p/--provider`, and
  `-t/--task` to hand the agent an initial task (interactive session already working on it,
  visible in `sloop ps`). The model is forwarded to the CLI as-is, never validated.
  `--split` runs several tools side by side; `-w` targets a registered workspace.

### Cross-repo fleet
- `sloop ps` — every running agent across all your repos; agents waiting on you float
  to the top. `--watch` live-monitors and alerts (terminal bell + desktop notify);
  `--waiting`, `--all`, and `ps <#>` to jump.
- Interactive control center: jump, send a reply, or kill the highlighted session in place.
- `sloop ls` — registered workspaces with live status; Enter attaches to a running session
  or launches the workspace's default tool right there.
- `sloop send` (`--waiting`/`--all` broadcast), `sloop kill`, `sloop attach`,
  `sloop run --split`.

### Provider-aware
- Tools are declarative adapter manifests (detect/launch/context/skills/hooks/scaffold);
  adding a CLI is adding a file. Built-ins: Claude, Cursor, Codex, Copilot, Gemini,
  Antigravity. `sloop tools` shows the capability matrix.
- `sloop hooks` wires each tool's own hooks for authoritative status (Claude, Gemini &
  Cursor auto-install; others print-and-paste).
- `sloop skills new` / `add` — reusable skills shared across every tool. `.sloop/skills.lock`
  records imported skills + source so `sloop skills update` re-fetches reproducibly.

### Cross-platform & DX
- Multiplexer-agnostic: tmux on macOS/Linux, **psmux** on native Windows (`SLOOP_MUX`
  to override).
- Contextual education hints (English + Vietnamese); `sloop hints on|off`.
- Dynamic shell completion; local SQLite registry + history (WAL + migrations).

### Foundation
- Single CGO-free Go binary, no daemon, no cloud.
