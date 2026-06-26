# Sloop — Hands-on Usage (dogfooding guide)

A practical, example-driven walkthrough. For the "what & why", see `README.md`.

> **Requirements:** Go 1.26 to build. **tmux** (macOS/Linux) for the orchestration features
> (`run --split`, `ps`). Plain `sloop run` works without tmux (single tool, no multiplexing).
> The AI tool binaries you target (`claude`, `cursor`/`agent`, `codex`, `copilot`, `gemini`) must
> be installed for sloop to launch them.

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
  config.yaml        # enabled tools + default tool (auto-detected)
  skills/            # reusable *.md skills
  vault/             # personal notes (not delivered to tools)
  profiles/          # one per enabled tool
  .gitignore
```

Now edit `AGENTS.md` with your project guidance (overview, conventions, build/test commands).
Commit `AGENTS.md` and `.sloop/` to git — they're shared with your team.

> _(planned, not yet built)_ `sloop init --scan` will pre-fill `AGENTS.md` from the codebase
> (language, build/test commands, layout) instead of an empty starter.

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
⚓ AI fleet — 2 running

  1   my-service       cursor    ○ idle · active just now
      └ Running tests... 12 passed
  2   webapp           claude    ● attached · active 3m ago
      └ │ Waiting for your approval to edit main.go

jump: sloop ps <#>   (switches client if you're already in tmux)
```

- The `└` line is each session's **last terminal output** — glance to see which agent needs you.
- Jump straight to one:

```sh
sloop ps 2          # attach (or switch-client if you're already inside tmux)
sloop attach webapp__claude   # attach by full session name
```

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
⚓ my-service · claude · agents:ok · ctx:ok · skills:linked
```

Delivery is **create-if-missing**: sloop never overwrites a file you hand-authored (it warns).
`AGENTS.md` is always yours — sync/repair never touch it.

---

## 5. Skills (shared across every tool)

```sh
sloop skill new code-review     # scaffolds .sloop/skills/code-review.md
```

Write the skill in `.sloop/skills/*.md`. Because it's **symlinked** into each tool's skills dir,
every tool sees the same set — edit once, available everywhere.

---

## 6. Inventory

```sh
sloop tools     # adapters + install status
sloop ls        # registered workspaces + recent sessions (history, from SQLite)
sloop doctor    # environment health (tools, tmux, mode)
```

(`ls` = registry/history; `ps` = what's live right now.)

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

- Orchestration (`ps`, `run --split`, `attach`) needs **tmux** → macOS/Linux only. Windows
  multiplexer support is not built yet.
- The `ps` glance is a heuristic (last terminal line). Precise "waiting / working / done" status
  via each tool's own hooks is a planned next step.
- `sloop` launches tool binaries directly (no shell), so an adapter's `launch:` must be a plain
  command, not a shell pipeline.
