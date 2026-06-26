# ⚓ Sloop

> **A local-first developer workspace for AI coding tools.**

Sloop is a lightweight CLI that improves the developer experience (DX) of working with AI coding tools such as Claude Code, Cursor CLI, Codex CLI, GitHub Copilot CLI, Gemini CLI, and future local agents.

Instead of replacing existing tools, Sloop provides a shared workspace layer that keeps your AI workflows organized, contextual, and reusable.

---

## Why Sloop

Modern AI coding tools are powerful, but using several of them together quickly becomes fragmented.

Developers often need to:

- manage multiple terminal sessions
- switch between different AI tools
- repeatedly provide project context
- remember prompts and workflows
- maintain personal knowledge and reusable skills

Sloop reduces this friction by introducing a lightweight workspace layer above your existing AI CLI tools.

---

## What Sloop Provides

- **One canonical context, every tool sees it** — write your project guidance once in
  `AGENTS.md` at the repo root. Tools that read `AGENTS.md` natively (Cursor, Codex, Copilot)
  use it directly; tools with their own file (Claude → `CLAUDE.md`, Gemini → `GEMINI.md`) get a
  thin **pointer** file that redirects to `AGENTS.md`.
- **Zero-copy skills** — `.sloop/skills/*.md` is **symlinked** into each tool's native skills
  directory (e.g. `.claude/skills`). Editing either side edits the same files — no duplication,
  no drift.
- **Never clobbers your files** — delivery is **create-if-missing**. Sloop writes what's absent
  and leaves anything you hand-authored untouched (it warns instead of overwriting). There is no
  `--force`; recovery is opt-in via `sync --repair`, which backs your file aside before replacing.
- **One-command launch** — `sloop run claude` cd's into the workspace and starts the tool with
  context already in place. No juggling terminals, paths, or flags.
- **Workspaces from anywhere** — register projects once; jump to any of them with
  `sloop run -w <name>`, with optional tmux multiplexing.
- **Personal vault (second brain)** — `.sloop/vault/*.md` for your own notes (kept local to the
  workspace; not delivered to tools).
- **Session history** — a local SQLite log of what you launched, where, and when.
- **Local-first** — a single binary, no daemon, no cloud, no required services, CGO-free.

---

## Mental Model

```
                Developer
                    │
                    ▼
                  Sloop
                    │
         AGENTS.md (canonical)  +  .sloop/skills (symlinked)
                    │
               ┌────┼─────────────────────┐
               ▼    ▼                     ▼
            Claude  Cursor CLI      Other AI CLIs
        (CLAUDE.md →  (reads        (pointer or native
         AGENTS.md)   AGENTS.md)     per adapter)
```

Sloop focuses on the developer experience, while existing AI tools remain responsible for code generation and reasoning.

---

## How It Works

`AGENTS.md` (at the repo root) is the **canonical context** — you author it, it's committed to
git. Sloop's job is to make each tool see it, without copying or clobbering.

`sloop run <tool|profile>`:

1. **Resolve the workspace** — walk up to the directory holding `.sloop/`, or use `-w <name>`
   from the global registry.
2. **Resolve the target** — a tool, or a profile (a named tool binding).
3. **Ensure `AGENTS.md` exists** — create a starter if it's missing (never overwrites yours).
4. **Deliver** —
   - *pointer-mode tools* (Claude, Gemini): write a thin pointer file (`CLAUDE.md`/`GEMINI.md`)
     that redirects to `AGENTS.md`, only if absent;
   - *native-mode tools* (Cursor, Codex, Copilot): nothing to write — they read `AGENTS.md`;
   - *skills*: symlink `.sloop/skills/` into the tool's native skills dir (relative link, so it
     survives moving/renaming the repo; falls back to a copy where symlinks aren't available).
5. **Record the session** in SQLite (best-effort; a write failure warns, never blocks the launch).
6. **Launch** the tool at the workspace root — through tmux if present, otherwise plain exec.
   Anything after `--` is passed straight through to the tool (`sloop run claude -- --model opus`).

`sloop sync [tool | --all]` runs steps 1–4 only (no launch), to (re)deliver context and skills.

Sloop is a **single binary, no daemon**. tmux is an optional enhancement, never required.

### Commands

```
sloop init [--name <n>]                       # scaffold AGENTS.md + .sloop/, register workspace, auto-enable detected tools
sloop sync [tool | --all] [--repair]          # deliver context pointers + skills symlinks (--all: every enabled tool)
sloop run [tool|profile] [-w <ws>] [-- <a>]   # sync + cd + launch (tmux-aware); args after -- go to the tool
sloop ls                                      # list workspaces + recent sessions
sloop attach <name>                           # attach a tmux session (if tmux present)
sloop tools                                   # list adapters + install status
sloop doctor                                  # environment health check
sloop status                                  # one-line Sloop statusline
sloop skill new <name>                        # scaffold a reusable skill in .sloop/skills
```

`sync --repair` is non-destructive: when a target is occupied by a file Sloop didn't create, it
renames the occupant aside (`<name>.sloopbak-<timestamp>`) and then writes Sloop's artifact. It
never deletes, and never touches `AGENTS.md`.

---

## Design Principles

- Developer experience first
- Local-first by default
- Build on existing AI tools
- Canonical source, never clobbered
- Reuse knowledge instead of repeating prompts
- Stay lightweight

---

## Workspace

A workspace is a project directory containing a `.sloop/` folder. `AGENTS.md` and `.sloop/` are
committed to git and shared with your team; the generated pointer files and skill symlinks live in
your tree — commit them or add them to your own `.gitignore`, as you prefer.

```
<project>/
  AGENTS.md           # canonical context (hand-authored) — the source of truth
  CLAUDE.md           # thin pointer → AGENTS.md (generated for Claude)
  .claude/skills      # symlink → ../.sloop/skills (generated)
  .sloop/
    config.yaml       # enabled tools, default tool
    skills/           # reusable *.md snippets — symlinked into each tool's skills dir
    vault/            # second-brain *.md notes (kept local; not delivered to tools)
    profiles/         # *.yaml launch presets (a named tool binding)
    .gitignore        # ignores .sloop/cache/ and *.local
```

Machine-local state lives separately under `~/.sloop/`: the workspaces registry, session
history (`sloop.db`), and any user-added adapter manifests (`~/.sloop/adapters/*.yaml`).

### Adapters

Tools are described by **declarative YAML manifests** — adding a tool is adding a file, not
editing Go. Built-ins are embedded; user adapters live in `~/.sloop/adapters/*.yaml`:

```yaml
name: Claude Code
detect: claude          # binary checked on PATH
launch: claude          # launch command
context:
  mode: pointer         # "pointer" (generate CLAUDE.md) | "native" (reads AGENTS.md)
  file: CLAUDE.md
skills:
  target: .claude/skills  # dir to symlink .sloop/skills into; empty = none
```

---

## Philosophy

Sloop does not compete with AI coding tools.

It connects them into a cohesive local development workflow, allowing developers to focus on building instead of managing terminals, prompts, and context.

---

## Vision

Sloop aims to become the developer experience layer for AI-native development—bringing together workspaces, context, skills, memory, and AI coding tools into one simple local workflow.
