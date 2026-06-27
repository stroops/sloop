# Sloop — Hands-on Usage (dogfooding guide)

A practical, example-driven walkthrough. For the "what & why", see `README.md`.

> **Requirements:** Go 1.26 to build. A **tmux-compatible multiplexer** for the orchestration
> features (`run --split`, `ps`, `send`, `attach`): **tmux** on macOS/Linux, or **[psmux]**
> (native, tmux-CLI-compatible) on Windows — no WSL needed. sloop auto-detects `tmux` then `psmux`;
> override with `SLOOP_MUX=<binary>`. Plain `sloop run` works without any multiplexer (single tool).
> The AI tool binaries you target (`claude`, `cursor`/`agent`, `codex`, `copilot`, `gemini`, `agy`)
> must be installed for sloop to launch them.
>
> [psmux]: https://github.com/psmux/psmux

---

## 0. Build

```sh
go build -o ~/bin/sloop ./cmd/sloop   # or: go install ./cmd/sloop
sloop doctor                          # check tools + tmux are detected
```

`sloop doctor` example:
```
Tools:
  ✓ claude 1.x
  ✗ codex
  ...
tmux: ✓ tmux 3.4
mode: ask
```

---

## 1. Set up a project (once per repo)

From inside an existing codebase:

```sh
cd ~/code/my-service
sloop init
```

This creates:
```
AGENTS.md            # canonical context — YOU write this (the source of truth)
.sloop/
  config.yaml        # version, enabled tools + default tool (auto-detected)
  skills/            # reusable *.md skills
  vault/             # personal notes (not delivered to tools)
  .gitignore
```

`init` also **delivers context for every detected tool** right away — it writes the pointer files
(`CLAUDE.md`, `GEMINI.md`, …) and links `.sloop/skills`, so the workspace is usable immediately
(no separate `sloop sync` needed first). It prints a per-tool summary of what it created. The personal
`vault/` is gitignored; `config.yaml` and `skills/` are committed (shared with your team).

Add `--scaffold` (`-S`) to also create each enabled tool's **standard folders** (e.g. `.claude/skills`,
`.claude/agents`, `.cursor/rules`, `.codex/skills`) — driven by the adapter manifests, so you start
from the provider's expected layout:

```sh
sloop init --scaffold
```

Or let sloop pre-fill `AGENTS.md` from the existing codebase (language, build/test/lint commands —
a `Makefile` target wins — project layout, and a README seed) instead of an empty starter:

```sh
sloop init --scan
```

Either way, then edit `AGENTS.md` with the rest of your guidance (overview, conventions). Commit
`AGENTS.md` and `.sloop/` to git — they're shared with your team. `--scan` is heuristic and offline
(no LLM, no API key); it never overwrites an existing `AGENTS.md`.

---

## 2. Launch a tool

```sh
sloop run                 # launch the default tool, context synced first
sloop run claude          # launch a specific tool
sloop run claude -- --model opus     # everything after -- is passed to the tool
sloop run -w my-service claude       # target a registered workspace from anywhere (no cd)
```

What `run` does: ensures `AGENTS.md`, writes pointer files (e.g. `CLAUDE.md` → `AGENTS.md`),
symlinks `.sloop/skills` into the tool's skills dir, records the session, then launches —
inside a tmux session named `<workspace>__<tool>` when tmux is present.

### Side-by-side panes (the orchestration win)

```sh
sloop run --split claude cursor      # one tmux window, two panes, same repo, side by side
sloop run --split                    # just the default tool (one pane)
```

Great for running two agents on the same code and comparing, or a coder + a reviewer.

---

## 3. Manage the fleet (`sloop ps`)

The answer to "too many AI windows". From anywhere:

```sh
sloop ps
```

```
⚓ AI fleet — 2 running, 1 waiting on you

  1   webapp           claude    ◆ waiting on you · active 3m ago
      └ │ Waiting for your approval to edit main.go
  2   my-service       cursor    ▸ working · active just now
      └ Running tests... 12 passed

jump: sloop ps <#>   ·   send: sloop send <#> "msg"
```

- Each session is classified from **its own terminal** (non-invasive — sloop never
  reads the provider): `◆ waiting on you` (blocked on an approval/question), `▸ working`,
  `○ idle`, or `● attached`. **Sessions waiting on you float to the top** so you see who
  needs you first. Install the Claude hooks (below) to make this status **authoritative**
  instead of a heuristic.
- The `└` line is the session's last terminal output (the glance).
- On a real terminal, `sloop ps` opens an interactive menu — `↑/↓` (or `j/k`) to move,
  `Enter` to jump, `q`/`Esc` to quit. Piped/CI prints the plain listing above. Colors honor
  `NO_COLOR` and `--no-color`, and turn off automatically when output isn't a terminal.
- Jump straight to one:

```sh
sloop ps 2          # attach (or switch-client if you're already inside tmux)
sloop attach webapp__claude   # attach by full session name
```

### Live monitor (`--watch`) and filtering (`--waiting`)

```sh
sloop ps --waiting             # show only agents waiting on you
sloop ps -f                    # follow live (alias of --watch): refresh every 2s, ring the bell
sloop ps -f -n 5s              # custom interval
sloop ps -f --notify           # also fire a desktop notification on new waiting agents
sloop ps -f --waiting          # monitor, but only list those that need you
```

> `-f` (follow) is the short for `--watch`. `-w` is reserved for `--workspace` everywhere
> (`run`/`sync`), so it is intentionally not a `ps` shorthand.

### Precise status via Claude's own hooks

The pane heuristic is a good guess; Claude's hooks make it exact. Install once per repo:

```sh
sloop hooks install     # merges into .claude/settings.local.json (idempotent, never clobbers)
sloop hooks print       # or print the JSON snippet to add by hand
```

This registers three of Claude's documented hooks — `UserPromptSubmit` → working,
`Notification` → waiting on you, `Stop` → idle — each calling `sloop hook <state>`, which
records the session's status under `~/.sloop/state`. `sloop ps` then **prefers that marker**
over the heuristic (falling back to the heuristic if no fresh marker exists). This stays
within the provider's rules: Claude calls sloop through its **own** hook mechanism; sloop
never intercepts or injects. Markers older than 15 min are ignored so a crashed session
can't get stuck "waiting".

**Multi-provider:** every major CLI now has a hook/notify system. `sloop hooks list` shows the
matrix, and `sloop hooks print <tool>` prints the exact event → `sloop hook <state>` wiring for
each (Claude and Gemini auto-install; Cursor/Copilot/Codex are print-and-paste for now):

```
sloop hooks list
TOOL      AUTO-INSTALL  CONFIG
claude    yes           .claude/settings.local.json
gemini    yes           .gemini/settings.json
cursor    print+paste   .cursor/hooks.json
copilot   print+paste   ~/.copilot/hooks/notification-hooks.json
codex     print+paste   ~/.codex/config.toml  (notify = [...])
```

Hook knowledge lives in each tool's adapter manifest (see `docs/ADAPTERS.md`), so adding a provider
is "drop one yaml". `sloop tools` shows the full capability matrix (context / skills / hooks).

### Whole cross-repo board (`ps --all`)

`sloop ps` lists what's live; `sloop ps --all` also lists registered workspaces that **aren't**
running, with their repo path — the full picture of every project, not just the running ones:

```sh
sloop ps --all
```
```
⚓ AI fleet — 1 running
  1   api          claude    ◆ waiting on you · active 2m ago

Known workspaces (not running):
  ○ web              ~/code/web
  ○ infra            ~/code/infra

start one: sloop run -w <name>
```

`--watch` turns `ps` from a snapshot into a live board: it re-renders on the interval and,
whenever a session **newly** starts waiting on you, rings the terminal bell (and, with
`--notify`, shows an OS notification via `osascript`/`notify-send`). Captures run concurrently,
so even a large fleet refreshes quickly. Ctrl-C to stop.

### Answer an agent without attaching (`sloop send`)

When `ps` shows an agent `◆ waiting on you`, reply without switching to it:

```sh
sloop send 1 "yes, go ahead"                    # by fleet number (from ps)
sloop send webapp__claude "use the opus model"  # by full session name
sloop send webapp "run the tests"               # by workspace (if it has one session)
```

`send` types the message into that session's pane and presses Enter — exactly as if you
typed it yourself (via `tmux send-keys`; the provider is never intercepted). Great for
unblocking an agent across repos without losing your place.

---

## 4. Context delivery without launching

```sh
sloop sync               # (re)deliver for the default tool
sloop sync claude        # a specific tool
sloop sync --all         # every enabled tool
sloop sync --repair      # if a file/dir you didn't create occupies a target,
                         # move it aside (*.sloopbak-<ts>) and write sloop's — never deletes
sloop status             # one-line delivery state
```

`sloop status` example:
```
⚓ my-service  ~/code/my-service
  tools:    claude*, codex, cursor, gemini  (* default)
  context:  AGENTS.md ok · CLAUDE.md ok
  skills:   3 in .sloop/skills · linked: claude
  hooks:    auto: claude, gemini  (sloop hooks list)
  running:  2 sessions  (sloop ps)
```

Delivery is **create-if-missing**: sloop never overwrites a file you hand-authored (it warns).
`AGENTS.md` is always yours — sync/repair never touch it.

---

## 5. Skills (shared across every tool)

```sh
sloop skills new code-review                                  # scaffold + link into your tools
sloop skills add https://example.com/review.md               # import from a URL
sloop skills add https://github.com/o/r/blob/main/review.md  # GitHub blob URL (auto-raw)
sloop skills add <url> --name custom-name                    # override the derived name
```

Skills live in `.sloop/skills/*.md` and are **symlinked** into each tool's skills dir — write or
import once, every tool sees the same set. `skills new`/`add` link them in automatically (`skill`
and `sk` are aliases). If a tool isn't linked yet, run `sloop sync`.

---

## 6. Inventory

```sh
sloop tools     # adapters + install status
sloop ls        # registered workspaces + recent sessions (history, from SQLite)
sloop doctor    # environment health (tools, tmux, mode)
```

(`ls` = registry/history; `ps` = what's live right now.)

---

## 7. Shell completion (autocomplete)

Install once, then Tab-complete commands, flags **and live values**:

```sh
# zsh (add to ~/.zshrc, or drop into a completions dir)
source <(sloop completion zsh)
# bash
source <(sloop completion bash)
# fish
sloop completion fish | source
```

Completion is **dynamic**: `sloop run <Tab>` lists your tools, `-w <Tab>` lists registered
workspaces, and `sloop ps`/`send`/`attach <Tab>` list the sessions running right now (with the
session name shown next to each `ps` number).

---

## Full dogfood walkthrough

```sh
# Two real projects
cd ~/code/api && sloop init && $EDITOR AGENTS.md      # fill in guidance
cd ~/code/web && sloop init && $EDITOR AGENTS.md

# Work the api with a coder + reviewer side by side
cd ~/code/api && sloop run --split claude cursor

# Detach (Ctrl-b d), start the web one too
cd ~/code/web && sloop run claude

# From anywhere, see the whole fleet and who needs you
sloop ps
sloop ps 1        # jump to the one that's waiting
```

**What to watch for while dogfooding:** does `sloop ps` + the glance line actually let you triage
"which agent needs me" across repos faster than flipping through tmux windows yourself? That's the
question that decides whether the orchestration direction is worth doubling down on.

---

## Limits (today)

- Orchestration (`ps`, `run --split`, `attach`, `send`) needs a tmux-compatible multiplexer:
  **tmux** on macOS/Linux, **psmux** on native Windows (auto-detected; `SLOOP_MUX` overrides).
  Without one, the orchestration commands are unavailable but everything else (`init`, `sync`,
  `skills`, `hooks`, `tools`, `status`, the registry DB) works fine. The multiplexer backend lives in
  `internal/tmux` behind the `runner.Runner` seam. **Note:** the psmux path is wired but not yet
  verified on a real Windows box — please report flag incompatibilities.
- The `ps` glance is a heuristic by default; install each tool's hooks (`sloop hooks install`) to make
  status authoritative.
- `sloop` launches tool binaries directly (no shell), so an adapter's `launch:` must be a plain
  command, not a shell pipeline.
