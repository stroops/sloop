# ⚓ Sloop

> **A local-first developer workspace for AI coding tools.**

Sloop is a lightweight CLI that improves the developer experience (DX) of working with AI coding tools such as Claude CLI, Cursor CLI, Aider, Antigravity CLI, and future local agents.

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

- **Canonical context, synced everywhere** — write project context once in `.sloop/`; Sloop
  generates each tool's native file (`CLAUDE.md`, `.cursor/rules`, `AGENTS.md`, …).
- **One-command launch** — `sloop run claude` cd's into the workspace and starts the tool
  with context already in place. No juggling terminals, paths, or flags.
- **Workspaces from anywhere** — register projects once; jump to any of them with
  `sloop run -w <name>`, with optional tmux multiplexing.
- **Reusable skills** — `.sloop/skills/*.md` snippets you can fold into a launch profile.
- **Personal vault (second brain)** — `.sloop/vault/*.md` knowledge referenced into context.
- **Session history** — a local SQLite log of what you launched, where, and when.
- **Local-first** — a single binary, no daemon, no cloud, no required services.

---

## Mental Model

```
                Developer
                    │
                    ▼
                  Sloop
                    │
        Workspace + Context + Skills
                    │
               ┌────┼─────────────────────┐
               ▼    ▼                     ▼
            Claude  Cursor CLI      Other AI CLIs
```

Sloop focuses on the developer experience, while existing AI tools remain responsible for code generation and reasoning.

---

## How It Works

`sloop run <tool|profile>`:

1. Resolves the workspace (walk up to `.sloop/`, or `-w <name>` from the global registry).
2. Assembles canonical context (`context/` + the profile's `skills/` + `vault/`).
3. Renders it into the tool's native file via a YAML adapter manifest.
4. Records the session in SQLite (best-effort).
5. Launches the tool at the workspace root — through tmux if present, otherwise plain exec.

Sloop is a **single binary, no daemon**. tmux is an optional enhancement, never required.

### Commands

```
sloop init [--name <n>]                # scaffold .sloop/, register workspace, auto-enable detected tools
sloop sync [tool | --all]              # regenerate native context files
sloop run [tool|profile] [-w <ws>]     # sync + cd + launch
sloop ls                               # list workspaces + recent sessions
sloop attach <name>                    # attach a tmux session (if tmux present)
sloop tools                            # list adapters + install status
sloop doctor                           # environment health check
sloop status                           # one-line Sloop statusline
```

---

## Design Principles

- Developer experience first
- Local-first by default
- Build on existing AI tools
- Keep workflows simple
- Reuse knowledge instead of repeating prompts
- Stay lightweight

---

## Workspace

A workspace is a project directory containing a `.sloop/` folder (committed to git, shared
with your team):

```
<project>/.sloop/
  config.yaml     # enabled tools, default tool
  context/        # canonical project context (markdown) — the source of truth
  skills/         # reusable *.md snippets
  vault/          # second-brain *.md notes
  profiles/       # launch presets (tool + selected context/skills/vault)
```

Generated native files (`CLAUDE.md`, `.cursor/rules`, `AGENTS.md`) are produced by `sloop
sync` and are gitignored.

Machine-local state lives separately under `~/.sloop/`: the workspaces registry, session
history (`sloop.db`), and any user-added adapter manifests. The exact contents will evolve
while remaining backward compatible.

---

## Philosophy

Sloop does not compete with AI coding tools.

It connects them into a cohesive local development workflow, allowing developers to focus on building instead of managing terminals, prompts, and context.

---

## Vision

Sloop aims to become the developer experience layer for AI-native development—bringing together workspaces, context, skills, memory, and AI coding tools into one simple local workflow.