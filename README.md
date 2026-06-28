# ⚓ Sloop

> **The local-first control layer for your AI coding CLIs.**
> One canonical context for every tool, and one cross-repo view of every agent you're running.

[Documentation](docs/guide/USAGE.md) 

Sloop is a single, lightweight Go binary that sits **above** your AI coding tools — Claude Code,
Cursor CLI, Codex CLI, GitHub Copilot CLI, Gemini CLI, Google Antigravity, and future agents. It
doesn't replace them or proxy their models; it removes the friction of using several of them, across
several repos, at once.

---

## The problem

Running AI coding agents is powerful but quickly gets messy:

- **Context is duplicated** — each tool wants its own instructions file (`CLAUDE.md`, `GEMINI.md`, …),
  so the same project guidance gets copy-pasted and drifts out of sync.
- **Too many windows** — each agent runs in its own terminal. With a few repos open you lose track of
  *which agent is waiting for you* and which is still working.
- **Per-tool knobs everywhere** — skills, hooks, and standard folders differ per provider, so setup is
  ad-hoc and hard to share with a team.

Concretely, running two AI CLIs (say Claude and Cursor's `agent`) in one project today means juggling
tmux by hand:

```
# The manual way — fiddly and easy to mix up
Terminal 1 → tmux new -s claude_api → cd ~/code/api → claude
Terminal 2 → tmux new -s cursor_api → cd ~/code/api → agent
# …now switch between them yourself, and remember which window is which,
#    across every repo, while CLAUDE.md and AGENTS.md drift apart.
```

```
# With sloop — named sessions per workspace+tool, one fleet view, one context
cd ~/code/api
sloop run claude            # session api__claude, sees AGENTS.md
sloop run cursor            # session api__cursor, same context
sloop ps                    # one board: who's waiting, who's working — jump/reply/kill in place
```

Sloop fixes these with three ideas:

1. **Portable context** — write guidance once in `AGENTS.md`; every tool reads it (natively, or via a
   thin pointer file). Skills are written once and **symlinked** into each tool.
2. **Cross-repo fleet** — `sloop ps` shows every running agent across all your repos, floats the ones
   **waiting on you** to the top, and lets you jump / reply / kill in place.
3. **Provider-aware by construction** — everything a tool needs (detect, launch, context, skills,
   hooks, standard folders) lives in one declarative adapter manifest. Adding a CLI = adding one file.

**Local-first:** one binary, no daemon, no cloud, no required services, CGO-free.

---

## Prerequisites

- **To run sloop:** nothing — it's a single static binary. (Building from source needs **Go 1.26+**.)
- **For the orchestration features** (`ps`, `run --split`, `send`, `attach`, `kill`): a
  tmux-compatible multiplexer — **[tmux]** on macOS/Linux/WSL, or **[psmux]** on native Windows
  (auto-detected; override with `SLOOP_MUX`). Everything else works without it.
- **The AI CLIs you actually use** (`claude`, `cursor`/`agent`, `codex`, `copilot`, `gemini`,
  `antigravity`) must be installed for sloop to launch them.

[tmux]: https://github.com/tmux/tmux/wiki/Installing
[psmux]: https://github.com/psmux/psmux

---

## Installation

```sh
# Homebrew (macOS / Linux)
brew install stroops/tap/sloop
brew upgrade sloop           # to update later

# With Go (always works)
go install github.com/stroops/sloop/cmd/sloop@latest

# From source
git clone https://github.com/stroops/sloop && cd sloop
make build                   # → ./sloop   (or: make install)

# Prebuilt binaries: see the GitHub Releases page.
```

Verify and (optionally) enable shell completion:

```sh
sloop doctor                 # check tools + multiplexer are detected
source <(sloop completion zsh)   # bash / fish also supported
```

---

## Quick start

```sh
cd ~/code/my-service
sloop init                   # scaffold AGENTS.md + .sloop/, deliver CLAUDE.md, register the workspace
$EDITOR AGENTS.md            # write your project guidance once

sloop run claude             # sync context, then launch claude (inside tmux if present)
sloop hooks install          # let `sloop ps` know exactly when an agent is waiting

# …open more agents in more repos…
sloop ps                     # the whole fleet: who's waiting, who's working — across every repo
```

In the `sloop ps` menu: `↑/↓` move · `Enter` jump in · `s` reply · `x` kill · `q` quit.

---

## Commands (the menu)

`alias` = command shortcut · `-x` = short flag.

| Command | Alias | Flags (short) | What it does |
|---|---|---|---|
| `init` | — | `-s/--scan`, `-S/--scaffold` | Scaffold `AGENTS.md` + `.sloop/`, deliver pointers, register the workspace. `--scan` pre-fills `AGENTS.md` from the codebase; `--scaffold` creates each tool's standard folders. |
| `run [tool]` | `r` | `-w/--workspace`, `--split`, `-- <args>` | Sync context, then launch a tool (in tmux if present). `--split` runs several side by side; `-w` targets a registered workspace from anywhere. |
| `sync [tool]` | `s` | `-a/--all`, `-r/--repair`, `-w/--workspace` | (Re)deliver pointer files + skills symlinks without launching. `--repair` safely moves a foreign file aside (never deletes). |
| `ps [#]` | — | `-f/--watch`, `-n/--interval`, `--waiting`, `--notify`, `--all` | The cross-repo fleet. Reads what each agent is asking and shows answer keys; `<#>` jumps; `-f` live-monitors + alerts. |
| `approve <target>` | — | `--waiting`, `--all`, `--yes` | Send the Yes/Approve answer to waiting agent(s) — one-command approve. |
| `send <target> <msg>` | — | `--waiting`, `--all`, `--yes` | Reply to a running agent without attaching; `--waiting`/`--all` broadcast. |
| `kill <target>` | — | `--all`, `--waiting`, `--yes` | End session(s) — confirms (skip with `--yes` or global `-y`). |
| `adopt <tmux-session>` | — | `-w/--workspace`, `--as` | Bring an external tmux session (one you started yourself) into the fleet. |
| `restore` | — | `--resume`, `--yes` | Relaunch your recent agents (detached) after a reboot / tmux restart. `--resume` continues each tool's prior conversation. |
| `popup` / `popup setup` | `hud` | `--key` | Open the fleet `ps` as a floating tmux popup / HUD; `setup` binds a key. Needs tmux ≥ 3.2. |
| `statusline setup` | — | — | Show a session's live status (`⚓ repo tool ◆ waiting`) in its tmux status bar. |
| `attach [session]` | `a` | — | Attach to a session by name, or `sloop a` (no name) to pick from the fleet. |
| `skills new\|add <…>` | `sk`, `skill` | `new`→`n`, `add`→`import` | Scaffold or import a reusable skill (shared across every tool). |
| `hooks install\|list\|print [tool]` | — | — | Wire a tool's own hooks so `ps` status is authoritative. |
| `tools` | — | — | Capability matrix (context / skills / hooks per tool). |
| `status` | `st` | — | One-screen workspace summary. |
| `ls` | — | — | Registered workspaces + recent sessions. |
| `doctor` | — | — | Environment health (tools, multiplexer, mode). |
| `hints [list\|on\|off]` | — | — | Contextual education tips (en/vi). |
| `completion <shell>` / `version` | — | — | Shell completion · version info. |

**Global flags** (any command): `-y/--auto` (assume yes), `--no-color`, `--no-input`, `--config <file>`,
`--debug` (log diagnostics to stderr; or set `SLOOP_DEBUG=1` — shows every multiplexer call sloop makes).

Full, example-driven walkthrough: **[docs/guide/USAGE.md](docs/guide/USAGE.md)**.

---

## How it works (in one breath)

`AGENTS.md` at the repo root is the **canonical context** — you author it, it's committed. `sloop run`
/ `sloop sync` make each tool see it without copying or clobbering: pointer-mode tools (Claude →
`CLAUDE.md`, Gemini → `GEMINI.md`) get a thin redirect file *only if absent*; native-mode tools
(Cursor, Codex, Copilot) read `AGENTS.md` directly; `.sloop/skills` is symlinked into each tool's
skills dir. Delivery is **create-if-missing** — sloop never overwrites a file you hand-authored.
Launch happens in a tmux/psmux session named `<workspace>__<tool>`, which is what makes the fleet
view, `send`, and `attach` possible.

Architecture & internals: **[docs/reference/ARCHITECTURE.md](docs/reference/ARCHITECTURE.md)**.

---

## Workspace layout

```
<project>/
  AGENTS.md           # canonical context (hand-authored) — the source of truth
  CLAUDE.md           # thin pointer → AGENTS.md (generated, create-if-missing)
  .claude/skills      # symlink → ../.sloop/skills (generated)
  .sloop/
    config.yaml       # version, enabled tools, default tool
    skills/           # reusable *.md skills — symlinked into each tool's skills dir (committed)
    vault/            # personal notes — kept local, gitignored
    .gitignore
```

Machine-local state lives under `~/.sloop/`: the workspaces registry + session history (`sloop.db`,
SQLite with WAL), hook status markers (`state/`), and any user adapter manifests (`adapters/*.yaml`).
Config layering is documented in **[docs/reference/CONFIG.md](docs/reference/CONFIG.md)**.

**Across restarts.** tmux sessions live in memory, so a reboot ends them (sloop is not tmux-resurrect).
What sloop keeps is the **registry**: after a restart your workspaces and recent sessions are still
there, so `sloop ls` / `sloop ps --all` show them and `sloop run -w <name>` relaunches from anywhere.
The agent's own conversation is the provider's to resume (e.g. `claude` continues a prior session).

---

## Provider-aware adapters

Every tool is a declarative YAML manifest — adding a CLI is adding a file, never editing Go. Built-ins
are embedded; user adapters/overrides live in `~/.sloop/adapters/*.yaml`. `sloop tools` shows the
capability matrix. See **[docs/reference/ADAPTERS.md](docs/reference/ADAPTERS.md)** for the contract.

```yaml
name: Claude Code
detect: claude
launch: claude
context: { mode: pointer, file: CLAUDE.md }   # or mode: native (reads AGENTS.md)
skills:  { target: .claude/skills }
hooks:                                          # status hooks for `sloop ps`
  install: settings-json
  config:  .claude/settings.local.json
  events:  { working: UserPromptSubmit, waiting: Notification, idle: Stop }
```

---

## Philosophy & non-goals

- **Build on existing tools** — sloop syncs context and orchestrates sessions; it never proxies an LLM
  or replaces a CLI.
- **Local-first & lightweight** — one CGO-free binary, no daemon, no cloud.
- **Canonical source, never clobbered** — `AGENTS.md` is yours; delivery is create-if-missing.
- **Not** an in-repo multi-agent orchestrator with worktrees/dashboards (tools like ntm and Claude
  Squad own that lane). Sloop's edge is **portable context + the cross-repo fleet view**.

## Docs

- [ROADMAP.md](ROADMAP.md) — pillars, what's next (v0.2.0 workflow hooks), how to contribute
- [docs/guide/USAGE.md](docs/guide/USAGE.md) — hands-on guide, every command with examples
- [docs/reference/CONFIG.md](docs/reference/CONFIG.md) — the three config layers (local / global / built-in)
- [docs/reference/ADAPTERS.md](docs/reference/ADAPTERS.md) — the provider-aware adapter contract
- [docs/design/run.md](docs/design/run.md) — `sloop run` design: CLI · model · effort resolution
- [docs/design/skills.md](docs/design/skills.md) — skills model, lockfile, registry roadmap
- [docs/design/hooks.md](docs/design/hooks.md) — status hooks today, workflow-hook design for v0.2.0
- [docs/reference/ARCHITECTURE.md](docs/reference/ARCHITECTURE.md) — packages, data flow, internals

## License

**MIT** — see [LICENSE](LICENSE).
