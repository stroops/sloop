# Hooks — status today, portable workflow automation next

AI CLIs expose **hooks**: events that fire at points in a session (per-session, per-turn, per
tool-call) where a command can observe, react, or block. Sloop uses hooks three ways — and keeping
them cleanly separated is the whole design:

1. **Status hooks** (shipped) — observe state for `sloop ps`.
2. **Workflow hooks** (v0.2.0 proposal) — run the user's portable automation.
3. **Context-inject hooks** (proposal) — deliver sloop's portable context / memory into a session at
   `SessionStart`, without writing into the tool's own context file.

> **Status hooks** ship today. **Workflow hooks** and **context-inject hooks** are proposals — this doc
> is the contract to review *before* they're built. Command reference for status hooks:
> [USAGE.md](../guide/USAGE.md); the install mechanism:
> [ADAPTERS.md §Hook install strategies](../reference/ADAPTERS.md).

## Status vs. workflow hooks (don't conflate them)

These two write into the *same* provider config, so the bulk of the design is keeping them apart.
(Context-inject hooks live on a different event and are read-only, so they don't share this hazard —
see their own section below.)

| | **Status hooks** (shipped) | **Workflow hooks** (v0.2.0) |
|---|---|---|
| Purpose | tell **sloop** the agent's state for `sloop ps` | run **the user's** automation (format, lint, policy) |
| Consumer | sloop's fleet view | the user / the team |
| Command wired in | `sloop hooks emit <state>` (reserved, hidden) | an arbitrary command from a library |
| Managed by | `sloop hooks install` (manifest-driven) | `sloop hooks add` (library + lockfile) |
| Tracking | derived from the manifest, recomputable | `.sloop/hooks.lock` |

### How the two stay non-conflicting

Both write into the *same* provider hook config (e.g. `.claude/settings.json`), so separation is by
construction:

1. **Reserved identity.** Sloop's status hook is always the exact command `sloop hooks emit <state>`.
   That prefix is sloop's namespace; the merge is idempotent and never duplicates it. (This is why the
   internal callback is namespaced as `hooks emit`, not a bare top-level `hook`.)
2. **Separate verbs.** `hooks install`/`uninstall` only ever touch the reserved status hook;
   `hooks add`/`remove`/`update` only ever touch lockfile-tracked workflow hooks. Neither command can
   see the other's entries.
3. **Separate tracking.** Status hooks are derived from the manifest (sloop can always recompute and
   reconcile them). Workflow hooks are recorded in `.sloop/hooks.lock`. Installing/removing one kind
   leaves the other untouched, and foreign (hand-written) hooks in the file are always preserved.

## Status hooks (shipped)

A provider calls `sloop hooks emit <waiting|working|idle>` from its own lifecycle events; `emit`
writes a short-lived marker under `~/.sloop/state` that `sloop ps` prefers over the screen heuristic
(15-min TTL, so it never goes stale). Per-provider event→state mapping and the install strategy live
in the adapter manifest. Auto-install today: **claude, gemini, cursor**; **copilot, codex** are
`print+paste` pending a matcher-aware model (below). See [ADAPTERS.md](../reference/ADAPTERS.md).

### Privacy: status hooks never read your session

This is a deliberate boundary and a promise we make to users. Sloop's status hook emits **one state
word** — `waiting` / `working` / `idle` plus a timestamp — into a local marker file. It does **not**:

- read the prompt or the model's response,
- read file contents or tool inputs,
- make any network call.

Contrast a typical lifecycle bridge (e.g. the `claude-code-warp` plugin), whose `Stop` hook *reads the
session, extracts the last prompt + response, and ships a summary* to an external notification center.
That is a real content-exfiltration surface; sloop's fleet awareness deliberately stays at the
state-enum level, so "sloop knows an agent is idle" never means "sloop read what it was doing."

The wiring is auditable end to end: `sloop hooks print` shows the exact line that gets added, the
command is literally `sloop hooks emit <state>`, it installs into `settings.local.json` (not
committed), the merge is idempotent, and any hand-written ("foreign") hooks in the file are preserved.

The only surface that *does* execute arbitrary code is **workflow hooks** (below) — which is exactly
why they are gated behind a lockfile + checksum + review-before-run, and why the internal status
callback is namespaced as `hooks emit` rather than a bare top-level `hook`.

## Workflow hooks (v0.2.0 proposal)

Turn hooks into a **portable automation library**: pick a hook, sloop installs it into the right
tool's own config — author once, run across tools and repos. Provider-respecting because it uses each
tool's *own* hook mechanism (or standard git hooks); sloop never intercepts.

### Categories

| Category | Typical event | Example |
|---|---|---|
| Quality gate | PostToolUse / afterFileEdit | run formatter/linter after an edit |
| Safety guard | PreToolUse / beforeShellExecution | block or confirm dangerous shell commands |
| Policy / governance | Stop, or git `commit-msg` | enforce a commit trailer, scan for secrets |
| Prompt / context rule | UserPromptSubmit | inject team standards into the prompt |
| Lifecycle / audit | SessionStart / SessionEnd | logging, notifications |
| Observability | Stop / Notification | (this is what status hooks already are) |

### Levels

A hook installs at one of two levels — the design must make this explicit:

- **Project** — `.claude/settings.json`, `.cursor/hooks.json`. Committed, team-shared, reproducible
  (pairs with `.sloop/hooks.lock`).
- **User / global** — `~/.claude/…`, `~/.cursor/…`. Personal, applies across every repo.

### Cross-tool mapping (sloop's value)

The same *logical* hook ("run formatter after an edit") maps to different events and schemas per tool
— Claude `PostToolUse` with `matcher: "Write|Edit"`, Cursor `afterFileEdit`, Copilot `notification`
with a matcher. A workflow-hook definition is therefore tool-agnostic and sloop **translates** it into
each tool's config — exactly the adapter-manifest pattern, extended with **matchers/cadences**. (The
status-hook schema has no matchers yet; copilot's status hook is the first real consumer that forces
adding them, so the schema gets designed against a concrete need rather than guessed.)

### Trust model

A hook **executes commands** and can **block actions** — distributing hooks is distributing code that
runs on a teammate's machine. So, unlike skills, the library needs:

- a **lockfile** (`.sloop/hooks.lock`) pinning `id` + `source` + **checksum** + target event/level;
- **review before run** — `hooks add` shows exactly what will be installed and where; nothing executes
  on `add`, only what the provider later invokes;
- provenance for any future registry (curated index, signing) before `hooks add <name>` from a remote
  source is allowed.

### Provider-respect boundary

- Prompt-rule hooks go through the provider's **own** `UserPromptSubmit` mechanism — never by
  intercepting or rewriting the user's prompt.
- Commit-policy hooks go through standard **git hooks** (`.git/hooks/commit-msg`), not by wrapping the
  AI tool.
- Each definition declares its target mechanism; sloop installs into that and nowhere else.

## Context-inject hooks — the memory/vault bridge (proposal)

Today sloop delivers portable context by **writing files**: `AGENTS.md` plus per-tool pointers
(`CLAUDE.md`, etc.). That works, but it has two limits: it touches files the provider considers its
own, and it's static — it can't reflect what the workspace has *learned* between sessions.

A `SessionStart` hook is the third delivery channel, and it's the one that unlocks the **memory /
vault** direction (design to live in `docs/design/memory.md`). The pattern is the
Superpowers one: register a single read-only hook that, at session start, prints sloop's relevant
context to stdout — the provider injects that into the session as additional context. Sloop never
rewrites the user's prompt and never edits the tool's `CLAUDE.md`; it speaks through the provider's
**own** `SessionStart` mechanism.

```text
SessionStart  →  `sloop context emit`  →  prints relevant vault/AGENTS context  →  provider injects it
```

Why this is the right shape for memory:

- **Dynamic, not static.** The hook runs each session, so it can surface what was learned/saved since
  last time — the static `AGENTS.md` can't.
- **Provider-respecting.** It's the *third* way the user asked about ("write into AGENTS.md, or
  CLAUDE.md, or through our hooks") — and the only one that doesn't write into the tool's files.
- **Same trust posture as status hooks.** Injection is **read-only and outbound-only**: sloop emits
  context it already owns under `.sloop/`; it does not read the session. No new exfiltration surface.
- **No bundled LLM.** Selection of "relevant" context stays plumbing (recency, the active workspace,
  simple matching). Any real retrieval is delegated to the agent — sloop is not the model.

Open design questions specific to this channel (to be settled in `docs/design/memory.md`, not here):

- What "relevant context" means at session start (whole vault vs. a recency/scoped slice) and the
  token budget for an injection.
- How memory gets *written* (a `SessionEnd`/`Stop` capture hook? an explicit `sloop remember`?) — the
  write path is a separate decision from this read path.
- Per-provider `SessionStart` support + payload shape in the adapter manifest (which CLIs expose it,
  and whether non-supporting tools fall back to the file-pointer delivery).

## Open questions (feedback wanted before the build)

- Library distribution: embedded built-ins in this repo first (review via PR, ship with the binary,
  zero infra), then `hooks add <url>` like skills, then a curated index? Or a registry sooner?
- Hook schema: how much of each tool's event model (matcher, cadence, payload, block/allow decision,
  `failClosed`, timeout) to model portably vs. pass through per tool.
- Conflict UX when a user already has a hand-written hook on the same event.
- Verification/signing bar for remote hooks.

## Contributing

- **Status hooks:** add/correct a tool's `hooks.*` in its adapter manifest — [ADAPTERS.md](../reference/ADAPTERS.md).
- **Workflow hooks (v0.2.0):** comment on the open questions above. The model is intentionally being
  settled in public *before* implementation — input now shapes the schema.
