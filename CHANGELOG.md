# Changelog

All notable changes to Sloop are documented here. This project adheres to
[Semantic Versioning](https://semver.org/).

## v0.1.3 - 2026-07-02

Every builtin AI CLI now reports precise fleet status, the status bar gained
model/context/rate-limit awareness, and `sloop new` launches an agent without
stealing your terminal.

### Copilot and Codex auto-install status hooks
- `sloop hooks install` now wires up **all five** hook-capable tools —
  claude, gemini, cursor, copilot, and codex — instead of stopping at
  `print+paste` for the last two.
- Copilot: sloop owns a dedicated `~/.copilot/hooks/sloop.json` (Copilot
  loads every `*.json` file in that directory), so install/uninstall never
  touches a hand-written hook file.
- Codex: sloop claims its single `notify` slot in `~/.codex/config.toml`
  only when it's free; if something else already runs there, `hooks install`
  prints copy-paste chaining instructions instead of overwriting it. A new
  hidden `sloop hooks notify <tool>` command routes Codex's one-program
  payload to the same status marker `hooks emit` already writes.
- The manifest's per-state event mapping (`hooks.events.*`) generalized from
  a plain string to `{event, matcher}`, so a tool's event can be
  discriminated by a sub-type (Copilot's `notification` fires for several —
  only `permission_prompt` means "waiting").
- Surveyed gemini-cli, Cursor CLI, and Copilot CLI for a native statusline
  mechanism (separate from hooks): none of the three expose one today, so
  that gap is now a documented, dated fact in each manifest rather than an
  open question.

### Status bar: model, context %, rate limit — without repeating a tool's own footer
- `sloop statusline install <tool>` (offered automatically by `sloop init`)
  registers a sloop feed on Claude/Antigravity's own statusline mechanism,
  chaining to any command you already had, and enriches the fleet marker
  with what the tool reports on every render.
- The tmux bar and `sloop ps` show model, context-window usage (as a
  block-character bar), and git branch at a glance — moved to the left
  side, since tmux truncates the right side first on a narrow terminal.
- New: 5-hour rate-limit usage, sourced from each provider's own convention
  — something no custom statusline script commonly surfaces, so it's always
  shown.
- Once a tool's own footer already reports model/context/branch (a feed is
  wired), the tmux bar stops repeating them and narrows to what only sloop
  knows (status, identity, rate limit); tools with no feed (codex/cursor)
  keep the full picture.
- One marker read and one pane capture now serve a whole render (previously
  up to three round trips); `~/.sloop/state` no longer grows forever —
  `sloop kill` removes a session's marker immediately, and `sloop ps` prunes
  stale ones after 24h.

### `sloop new` — launch an agent without attaching
- `new` is `run` minus the attach: same targets/flags/context sync, but your
  terminal stays free (`-a` attaches anyway, `-N` forces a fresh instance).
  `run` and `new` share one launch path so the two commands can't drift.
- Fixed a tmux target-matching bug: with `ws__claude__sec` running, a bare
  `-t ws__claude` could silently resolve to it instead of failing, so
  attach/send/kill risked hitting the wrong agent. Whole-session targets now
  always match exactly.

### Also
- Second-account profiles (`config_dir_env`, e.g. `CLAUDE_CONFIG_DIR`) are
  now respected everywhere a hook or statusline config path is resolved, not
  just the default account dir.

## v0.1.2 - 2026-06-30

A home base when you type `sloop` with nothing after it, plus first-class updates.

### A menu on bare `sloop`
- Running `sloop` with no arguments now opens an interactive launcher over the
  commands you reach for most (fleet, run, sync, check, init, doctor) instead of
  dumping help. It reuses the same arrow-key menu as `sloop ps`, and degrades to
  the plain help output when piped, in CI, under `--no-input`, or when you set
  `SLOOP_NO_MENU=1`.
- The launcher is drawn under a small masthead that also shows the current
  directory (which AI CLIs treat as the project root) and whether sloop is
  initialized there, highlights the row under the cursor, and carries a `↑/↓
  move · ⏎ select · q quit` guide along the bottom.
- Picking **Run** opens a second menu of the tools you have installed and your
  saved profiles — default tool first, with the working directory shown — so you
  choose what to launch instead of always getting the default. If the current
  directory isn't a sloop workspace yet, Run offers to initialize it for the
  chosen tool first (instead of failing with "no .sloop workspace").
- A **More** entry (or `m`) opens the full command reference — every command with
  a one-line description — and runs the one you pick; `q` returns to the menu.

### Consistent interactive screens
- `sloop ps` and `sloop ls` now share one menu component with the home launcher:
  the same cursor highlight, the navigation guide moved to the bottom, and a
  blank line of top padding, so every interactive screen looks and moves the
  same. Plain menus (home, Run picker) light the whole cursor row in the `❯`
  pointer's cyan; status-colored menus (ps, ls) light just the first column so
  each row keeps its waiting/working/idle color. Skipped under `NO_COLOR`.
- `sloop version` / `sloop --version` now report the real build version (it was
  blank in some builds because the version was wired after the command was
  defined).
- The `Using config file: …` line no longer prints on every command; it is now
  shown only under `--debug`.

### Update awareness
- New `sloop update` upgrades sloop the way it was installed: it detects a
  Homebrew install and runs `brew upgrade sloop`, a `go install` build and
  re-runs `go install …@latest`, and otherwise prints the right command rather
  than overwriting a binary it doesn't manage.
- sloop checks GitHub for newer releases in a detached background process —
  throttled to at most once a day and never on the critical path of a command —
  and shows a one-line `⬆ Update X available — run sloop update` notice on the
  menu and the `sloop ps` header. Dev/source builds never check or nag.

### Reach a waiting agent in one key
- Zero-config keys: `sloop run` now binds the peek and hud popups to a free tmux prefix key the first
  time it creates a session on a server (peek tries `j → a → f → p`, hud `h → g → G`), so you no longer
  have to remember `peek setup`. It only ever takes a key that is free, never clobbering your own
  binding, and records the choice in a tmux server option; opt out with `SLOOP_KEYS=0`.
- Actionable badge: the fleet-wide `⏳ N waiting` status badge now shows the exact keystroke that gets
  you there, e.g. `⏳ 1 waiting → Ctrl+b j`, using your real tmux prefix.
- Peek now floats with a titled border (`👀 peek · <session> — Ctrl+b d to close`) on tmux ≥ 3.3, so a
  peek reads as distinct from a full attach and tells you how to drop back.

### A second Claude account
- `sloop profile add --config-dir <dir>` sets up a profile that runs a tool under its own config dir
  (Claude via `CLAUDE_CONFIG_DIR`), so two logins live side by side. It creates the dir, symlinks the
  tool's shareable tooling by default, offers (opt-in) to share conversation history for cross-account
  resume, and never shares credentials so the logins stay separate. Launch it with `sloop run @<name>`;
  pass `--tool` to pick when several tools qualify.

### Enhancements
- `sloop ps` truncates rows to your actual terminal width instead of a fixed 72 columns.
- Clearer "how to get back" detach hint in the session status bar, and `sloop ls --prune` drops
  workspace registrations whose paths no longer exist on disk.
- Windows: install via Scoop.

## v0.1.1 - 2026-06-28

Run more than one agent per repo, peek into a waiting agent without leaving your
screen, and a sharper first-run experience, all on top of v0.1.0.

### Run multiple agents & accounts
- Named instances: `sloop run claude@review` / `-n/--name` runs a second agent of the same tool in one
  repo (session `<repo>__tool__instance`); `-N/--new` auto-names the next free slot (`claude·2`…).
- Profiles: save a tool + env once with `sloop profile add|ls|rm` (global `~/.sloop/config.yaml`) and
  launch it as `sloop run @<name>`, e.g. a different account via `CLAUDE_CONFIG_DIR`. `--env KEY=VAL`
  injects env one-off without a profile (`~`/`$VAR` expanded). The fleet view shows instances as
  `tool·instance`.

### Peek (overlay a waiting agent)
- `sloop peek` floats a waiting agent's live pane over your current screen so you can answer it and
  drop back without `switch-client` swapping your whole screen; `sloop peek setup` binds a key. Needs
  tmux ≥ 3.2. Every status bar gains a fleet-wide `⏳ N waiting` badge.

### Sharper onboarding (init / check / doctor)
- `sloop init` is provider-respecting: it asks about the tools you actually use (Claude-first order),
  prompts honestly and skips work that is already done, selects tools per workspace, and no longer
  offers to scaffold a provider's folders for it.
- `sloop check` gains more AI-readiness criteria, sourced from each tool's adapter manifest.
- `sloop doctor` groups and colors its output and explains the `mode` line.

## v0.1.0 - first release

The initial public release: the local-first control layer for your AI coding CLIs.

### Portable context
- `AGENTS.md` as the single canonical context; create-if-missing pointer files
  (`CLAUDE.md`, `GEMINI.md`, …); never overwrites your files.
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
- `sloop ps`: every running agent across all your repos; agents waiting on you float to
  the top. It reads each waiting agent's own prompt and shows the answer keys, so you can
  reply in one keystroke. `--watch` live-monitors and alerts (terminal bell + desktop
  notify); `--waiting`, `--all`, and `ps <#>` to jump.
- Interactive control center: arrow-key nav, `Enter` to attach, one key to answer a waiting
  agent (`y`/`n`/`1`…), `s` to send a line, `x` to kill, all in place; Esc/Ctrl-C cancel
  back to the fleet (never drops you to a shell). Provider display names, status colors, and
  column headers; the screen redraws cleanly with an action notice.
- `sloop approve`: send the affirmative answer to waiting agent(s) in one command
  (`--waiting`/`--all`).
- `sloop ls`: registered workspaces with their live agents (colored by status, the same
  language as `ps`); `Enter` attaches, `r` launches the default tool, `s` opens a shell, `c`
  copies a `cd`.
- `sloop attach` (`a`): by session name, or with no argument a fleet picker that matches
  the `ps` view. `sloop adopt` brings an external tmux session into the fleet.
- `sloop restore`: relaunch the agents you were recently running after a reboot / tmux
  restart, each detached; `--resume` continues each tool's prior conversation where supported.
- `sloop popup` / `sloop hud`: the fleet as a floating tmux popup (HUD); `popup setup` binds
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
- `sloop skills new` / `add`: reusable skills shared across every tool. `.sloop/skills.lock`
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
