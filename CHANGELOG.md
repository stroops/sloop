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
- `sloop ps` — every running agent across all your repos; agents waiting on you float to
  the top. It reads each waiting agent's own prompt and shows the answer keys, so you can
  reply in one keystroke. `--watch` live-monitors and alerts (terminal bell + desktop
  notify); `--waiting`, `--all`, and `ps <#>` to jump.
- Interactive control center: arrow-key nav, `Enter` to attach, one key to answer a waiting
  agent (`y`/`n`/`1`…), `s` to send a line, `x` to kill — all in place, and Esc/Ctrl-C cancel
  back to the fleet (never drops you to a shell). Provider display names, status colors, and
  column headers; the screen redraws cleanly with an action notice.
- `sloop approve` — send the affirmative answer to waiting agent(s) in one command
  (`--waiting`/`--all`).
- `sloop ls` — registered workspaces with their live agents (colored by status, the same
  language as `ps`); `Enter` attaches, `r` launches the default tool, `s` opens a shell, `c`
  copies a `cd`.
- `sloop attach` (`a`) — by session name, or with no argument a fleet picker that matches
  the `ps` view. `sloop adopt` brings an external tmux session into the fleet.
- `sloop restore` — relaunch the agents you were recently running after a reboot / tmux
  restart, each detached; `--resume` continues each tool's prior conversation where supported.
- `sloop popup` / `sloop hud` — the fleet as a floating tmux popup (HUD); `popup setup` binds
  a key (needs tmux ≥ 3.2).
- Per-session status bar (`sloop statusline`) shows live `⚓ repo tool ◆ waiting` plus a
  persistent detach tip using your real tmux prefix; set only per session (never touches
  `~/.tmux.conf`), and `SLOOP_STATUSLINE=0` leaves a custom bar untouched.
- `sloop send` (`--waiting`/`--all` broadcast), `sloop kill`, `sloop run --split`.

### Provider-aware
- Tools are declarative adapter manifests (detect/launch/context/skills/hooks/scaffold);
  adding a CLI is adding a file. Built-ins: Claude, Cursor, Codex, Copilot, Gemini,
  Antigravity. `sloop tools` shows the capability matrix.
- `sloop hooks` wires each tool's own **status** hooks for authoritative `sloop ps` state
  (Claude, Gemini & Cursor auto-install; others print-and-paste). The reserved callback is
  `sloop hooks emit <state>`, keeping the namespace clear for the v0.2.0 workflow-hook library.
- `sloop skills new` / `add` — reusable skills shared across every tool. `.sloop/skills.lock`
  records imported skills + source so `sloop skills update` re-fetches reproducibly.

### Cross-platform & DX
- Multiplexer-agnostic: tmux on macOS/Linux, **psmux** on native Windows (`SLOOP_MUX`
  to override).
- Contextual education hints (English + Vietnamese); `sloop hints on|off`.
- Dynamic shell completion; local SQLite registry + history (WAL + migrations).
- `--debug` (or `SLOOP_DEBUG=1`) logs diagnostics via `log/slog` to stderr, including every
  multiplexer call sloop makes.

### Foundation
- Single CGO-free Go binary, no daemon, no cloud.
