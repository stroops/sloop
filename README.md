# ⚓ Sloop

> **The local-first control layer for your AI coding CLIs.**
> One canonical context for every tool, and one cross-repo view of every agent you're running.

[Documentation](docs/guide/USAGE.md)

Sloop is a single, lightweight Go binary that sits **above** your AI coding tools: Claude Code,
Cursor CLI, Codex CLI, GitHub Copilot CLI, Gemini CLI, Google Antigravity, and future agents. It
doesn't replace them or proxy their models; it removes the friction of using several of them, across
several repos, at once.

---

## The problem

Running AI coding agents is powerful but quickly gets messy:

- **Too many windows.** Each agent runs in its own terminal. With a few repos open you lose track of
  *which agent is waiting for you* and which is still working. This is the friction sloop tackles first.
- **Context is duplicated.** Each tool wants its own instructions file (`CLAUDE.md`, `GEMINI.md`, …),
  so the same project guidance gets copy-pasted and drifts out of sync.
- **Per-tool knobs everywhere.** Skills, hooks, and standard folders differ per provider, so setup is
  ad-hoc and hard to share with a team.

Concretely, running two AI CLIs (say Claude and Cursor's `agent`) in one project today means juggling
tmux by hand:

```
# The manual way: fiddly and easy to mix up
Terminal 1 → tmux new -s claude_api → cd ~/code/api → claude
Terminal 2 → tmux new -s cursor_api → cd ~/code/api → agent
# …now switch between them yourself, and remember which window is which,
#    across every repo, while CLAUDE.md and AGENTS.md drift apart.
```

```
# With sloop: named sessions per workspace+tool, one fleet view, one context
cd ~/code/api
sloop run claude            # session api__claude, sees AGENTS.md
sloop run cursor            # session api__cursor, same context
sloop ps                    # one board: who's waiting, who's working; jump/reply/kill in place
```

Sloop fixes these with three ideas:

1. **Cross-repo fleet.** `sloop ps` shows every running agent across all your repos, floats the ones
   **waiting on you** to the top, and lets you jump / reply / kill in place. This is the part we focus
   on first.
2. **Portable context.** Write your *project* guidance once in `AGENTS.md`; every tool reads it
   (natively, or via a thin pointer file). Skills are written once and **symlinked** into each tool.
   (This is project context, not agent memory; carrying a conversation across tools is a separate,
   later concern. See the roadmap.)
3. **Provider-aware by construction.** Everything a tool needs (detect, launch, context, skills,
   hooks, standard folders) lives in one declarative adapter manifest. Adding a CLI = adding one file.

**Local-first:** one binary, no daemon, no cloud, no required services, CGO-free.

---

## Prerequisites

- **To run sloop:** nothing. It's a single static binary. (Building from source needs **Go 1.26+**.)
- **For the orchestration features** (`ps`, `run --split`, `send`, `attach`, `kill`): a
  tmux-compatible multiplexer, either **[tmux]** on macOS/Linux/WSL or **[psmux]** on native Windows
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

# With Go (always works)
go install github.com/stroops/sloop/cmd/sloop@latest

# From source
git clone https://github.com/stroops/sloop && cd sloop
make build                   # → ./sloop   (or: make install)

# Prebuilt binaries: see the GitHub Releases page.
```

To update later, run **`sloop update`**: it detects how sloop was installed and
upgrades the same way (`brew upgrade sloop`, `go install …@latest`, …). sloop
also checks for new releases in the background — throttled, and never on the
critical path of a command — and shows a one-line notice on the menu and the
`sloop ps` header when one is available.

Running **`sloop`** with no arguments opens an interactive menu over the common
commands (fleet, run, sync, check, init, doctor). Set `SLOOP_NO_MENU=1` to keep
the plain help output instead.

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
sloop ps                     # the whole fleet: who's waiting, who's working, across every repo
```

In the `sloop ps` menu: `↑/↓` move · `Enter` jump in · `s` reply · `x` kill · `q` quit.

---

## Usage flow

The shape of a sloop day, end to end. A session is always a **workspace × a tool** (`api__claude`),
so sloop never opens a loose terminal and there's nothing untracked to lose. That one rule is what
makes the fleet board below possible.

```
You · one screen
  │
  └─ sloop ps ───────────────── the fleet: every agent, across every repo
        │   the ones waiting on you float to the top
        │
        ├─ ⏎    jump into an agent → work → detach (prefix d); it keeps running
        ├─ y/1  answer a waiting agent right here, without attaching
        ├─ s    send a line      x  kill      q  quit
        └─ peek glance at a waiting agent over your current screen, then drop back

  start more agents from anywhere:
    sloop run claude            # this repo, this tool          → api__claude
    sloop run claude@review     # a 2nd agent in the same repo  → claude·review
    sloop run @work             # a saved profile (e.g. your work account)
```

**Profiles** are a small but handy piece. Most people keep two accounts (*personal* and *work*), so a
profile is usually just your second account, saved once and reachable from any repo:

```sh
sloop profile add work --config-dir ~/.claude-work   # the account's config dir
sloop run @work                                       # launch claude under it, in any repo
```

`--config-dir` is the friendly form: you name the directory, sloop maps it to the tool's own account
variable (`CLAUDE_CONFIG_DIR` for Claude), offers to **create it if missing**, and (since most setups
want the same plugins/skills/agents across both accounts) offers to **symlink that tooling** from your
main config. It can optionally share conversation history too (off by default), so a second account can
**resume the first's sessions** when one hits its rate limit. Your login (`.credentials.json`) is never
shared.

> Today this works for **Claude Code**, which selects an account via `CLAUDE_CONFIG_DIR`. Other CLIs
> follow as their account model is mapped into the adapter; the friendly flag stays the same.

The next section walks the same loop in more detail.

---

## Daily use: `sloop ps` is home base

Most days you live in **one screen**:

1. **`sloop ps`** is the fleet board. Agents *waiting on you* float to the top, colored by status.
2. **`Enter`** on a row jumps you straight into that agent (attaches its tmux session).
3. Work with the agent. To step out, **detach**: press your tmux prefix then `d` (`Ctrl+b d` by
   default, or `Ctrl+a d` if you remapped it). The agent **keeps running**; you land back at the fleet.
4. Back in `sloop ps`, triage without attaching: answer a waiting agent in one key (`y` / `1` …),
   `s` to send a line, `x` to kill, or `Enter` into the next one.

You never lose an agent: detaching hides it, it doesn't stop it. Every sloop session's status bar
shows **`⚓ detach: <prefix> d`** so you always know how to get back, and `sloop ps` is one keystroke
away. Rebooted? **`sloop restore`** relaunches the fleet. No name to type? **`sloop a`** opens the same
picker.

> The status bar is set **per session** (`set-option -t`); sloop never edits your `~/.tmux.conf`, and
> only touches `status-left`/`status-right` (not your colors/theme). Keep your own bar fully intact with
> `SLOOP_STATUSLINE=0`.

**Don't want to leave your screen?** From inside one agent, **`sloop peek`** floats a *waiting* agent's
live pane over your current work; answer it, close it, and you're exactly where you were (no
`switch-client` that swaps your whole screen). Bind a key with `sloop peek setup`. Needs tmux ≥ 3.2.

**A second agent, or a second account?** `sloop run claude@review` opens another claude in the same
repo (`claude·review` in the fleet); `sloop run claude --new` auto-names the next one (`claude·2`). For a
different account, save a profile once with `sloop profile add work --config-dir ~/.claude-work` (sloop
creates the dir and offers to share your tooling), then `sloop run @work`.

`sloop ls` is the companion view: your registered **workspaces** (running or not) with their live
agents, to launch (`r`), open a shell (`s`), `c` to copy a `cd`, or `Enter` to jump in.

---

## Commands (the menu)

`alias` = command shortcut · `-x` = short flag.

| Command | Alias | Flags (short) | What it does |
|---|---|---|---|
| `init` | - | `-s/--scan`, `-S/--scaffold` | Scaffold `AGENTS.md` + `.sloop/`, deliver pointers, register the workspace. `--scan` pre-fills `AGENTS.md` from the codebase; `--scaffold` creates each tool's standard folders. |
| `run [tool]` | `r` | `-n/--name`, `-N/--new`, `--env`, `-w/--workspace`, `--split`, `-- <args>` | Sync context, then launch a tool (in tmux if present). Run a **2nd agent of the same tool** with `tool@instance`, `-n/--name`, or `-N/--new` (auto-named `tool·2`…); a **different account** with a saved `@profile` or one-off `--env KEY=VAL`. `--split` runs several side by side; `-w` targets any registered workspace. |
| `profile add\|ls\|rm` | `prof` | `add --config-dir`, `add --tool`, `add --env` | Save reusable run profiles in `~/.sloop/config.yaml` (e.g. a 2nd account). `--config-dir ~/.claude-work` names an account dir; sloop maps it to the tool's account env var, offers to create it + share tooling. Launch one with `sloop run @<name>`. |
| `sync [tool]` | `s` | `-a/--all`, `-r/--repair`, `-w/--workspace` | (Re)deliver pointer files + skills symlinks without launching. `--repair` safely moves a foreign file aside (never deletes). |
| `ps [#]` | - | `-f/--watch`, `-n/--interval`, `--waiting`, `--notify`, `--all` | The cross-repo fleet. Reads what each agent is asking and shows answer keys; `<#>` jumps; `-f` live-monitors + alerts. |
| `approve <target>` | - | `--waiting`, `--all`, `--yes` | Send the Yes/Approve answer to waiting agent(s); one-command approve. |
| `send <target> <msg>` | - | `--waiting`, `--all`, `--yes` | Reply to a running agent without attaching; `--waiting`/`--all` broadcast. |
| `kill <target>` | - | `--all`, `--waiting`, `--yes` | End session(s); confirms (skip with `--yes` or global `-y`). |
| `adopt <tmux-session>` | - | `-w/--workspace`, `--as` | Bring an external tmux session (one you started yourself) into the fleet. |
| `restore` | - | `--resume`, `--yes` | Relaunch your recent agents (detached) after a reboot / tmux restart. `--resume` continues each tool's prior conversation. |
| `popup` / `popup setup` | `hud` | `--key` | Open the fleet `ps` as a floating tmux popup / HUD; `setup` binds a key. Needs tmux ≥ 3.2. |
| `peek [agent]` / `peek setup` | `pk` | `--key` | Overlay a waiting agent in a floating popup, then answer it and drop back **without switching your whole screen**. No arg → the lone waiting agent or a picker; `setup` binds a key. Needs tmux ≥ 3.2. |
| `statusline setup` | - | - | Show a session's live status (`⚓ repo tool ◆ waiting`) in its tmux status bar. |
| `attach [session]` | `a` | - | Attach to a session by name, or `sloop a` (no name) to pick from the fleet. |
| `skills new\|add <…>` | `sk`, `skill` | `new`→`n`, `add`→`import` | Scaffold or import a reusable skill (shared across every tool). |
| `hooks install\|list\|print [tool]` | - | - | Wire a tool's own hooks so `ps` status is authoritative. |
| `tools` | - | - | Capability matrix (context / skills / hooks per tool). |
| `status` | `st` | - | One-screen workspace summary. |
| `check` | - | - | AI-readiness checklist for the workspace (AGENTS.md, context, skills, hooks) with a fix command per gap. |
| `ls` | - | - | Registered workspaces + recent sessions. |
| `doctor` | - | - | Environment health (tools, multiplexer, mode). |
| `update` | - | - | Update sloop to the latest release, the same way it was installed (Homebrew / `go install` / manual). |
| `hints [list\|on\|off]` | - | - | Contextual education tips (en/vi). |
| `completion <shell>` / `version` | - | - | Shell completion · version info. |

**Global flags** (any command): `-y/--auto` (assume yes), `--no-color`, `--no-input`, `--config <file>`,
`--debug` (log diagnostics to stderr; or set `SLOOP_DEBUG=1`, which shows every multiplexer call sloop makes).

Full, example-driven walkthrough: **[docs/guide/USAGE.md](docs/guide/USAGE.md)**.

---

## How it works (in one breath)

`AGENTS.md` at the repo root is the **canonical context**: you author it, it's committed. `sloop run`
/ `sloop sync` make each tool see it without copying or clobbering: pointer-mode tools (Claude →
`CLAUDE.md`, Gemini → `GEMINI.md`) get a thin redirect file *only if absent*; native-mode tools
(Cursor, Codex, Copilot) read `AGENTS.md` directly; `.sloop/skills` is symlinked into each tool's
skills dir. Delivery is **create-if-missing**: sloop never overwrites a file you hand-authored.
Launch happens in a tmux/psmux session named `<workspace>__<tool>`, which is what makes the fleet
view, `send`, and `attach` possible.

Architecture & internals: **[docs/reference/ARCHITECTURE.md](docs/reference/ARCHITECTURE.md)**.

---

## Workspace layout

```
<project>/
  AGENTS.md           # canonical context (hand-authored), the source of truth
  CLAUDE.md           # thin pointer → AGENTS.md (generated, create-if-missing)
  .claude/skills      # symlink → ../.sloop/skills (generated)
  .sloop/
    config.yaml       # version, enabled tools, default tool
    skills/           # reusable *.md skills, symlinked into each tool's skills dir (committed)
    vault/            # personal notes, kept local (gitignored)
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

Every tool is a declarative YAML manifest: adding a CLI is adding a file, never editing Go. Built-ins
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

- **Build on existing tools.** Sloop syncs context and orchestrates sessions; it never proxies an LLM
  or replaces a CLI.
- **Local-first & lightweight.** One CGO-free binary, no daemon, no cloud.
- **Canonical source, never clobbered.** `AGENTS.md` is yours; delivery is create-if-missing.
- **Not** an in-repo multi-agent orchestrator with worktrees/dashboards (tools like ntm and Claude
  Squad own that lane). Sloop's edge is **portable context + the cross-repo fleet view**.

## Docs

- [ROADMAP.md](ROADMAP.md) - pillars, what's next (v0.2.0 workflow hooks), how to contribute
- [docs/guide/USAGE.md](docs/guide/USAGE.md) - hands-on guide, every command with examples
- [docs/reference/CONFIG.md](docs/reference/CONFIG.md) - the three config layers (local / global / built-in)
- [docs/reference/ADAPTERS.md](docs/reference/ADAPTERS.md) - the provider-aware adapter contract
- [docs/design/run.md](docs/design/run.md) - `sloop run` design: CLI · model · effort resolution
- [docs/design/profiles.md](docs/design/profiles.md) - named instances & profiles: a 2nd agent / 2nd account
- [docs/design/peek.md](docs/design/peek.md) - overlay a waiting agent without leaving your screen
- [docs/design/skills.md](docs/design/skills.md) - skills model, lockfile, registry roadmap
- [docs/design/hooks.md](docs/design/hooks.md) - status hooks today, workflow-hook design for v0.2.0
- [docs/reference/ARCHITECTURE.md](docs/reference/ARCHITECTURE.md) - packages, data flow, internals

## License

**MIT**. See [LICENSE](LICENSE).
