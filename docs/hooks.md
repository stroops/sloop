# Hooks — status today, portable workflow automation next

AI CLIs expose **hooks**: events that fire at points in a session (per-session, per-turn, per
tool-call) where a command can observe, react, or block. Sloop uses hooks two ways — and keeping the
two cleanly separated is the whole design.

> **Status hooks** ship today. **Workflow hooks** are a v0.2.0 proposal — this doc is the contract to
> review *before* it's built. Command reference for status hooks: [USAGE.md](USAGE.md); the install
> mechanism: [ADAPTERS.md §Hook install strategies](ADAPTERS.md).

## Two kinds of hook (don't conflate them)

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
`print+paste` pending a matcher-aware model (below). See [ADAPTERS.md](ADAPTERS.md).

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

## Open questions (feedback wanted before the build)

- Library distribution: embedded built-ins in this repo first (review via PR, ship with the binary,
  zero infra), then `hooks add <url>` like skills, then a curated index? Or a registry sooner?
- Hook schema: how much of each tool's event model (matcher, cadence, payload, block/allow decision,
  `failClosed`, timeout) to model portably vs. pass through per tool.
- Conflict UX when a user already has a hand-written hook on the same event.
- Verification/signing bar for remote hooks.

## Contributing

- **Status hooks:** add/correct a tool's `hooks.*` in its adapter manifest — [ADAPTERS.md](ADAPTERS.md).
- **Workflow hooks (v0.2.0):** comment on the open questions above. The model is intentionally being
  settled in public *before* implementation — input now shapes the schema.
