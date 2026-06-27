# Sloop Backlog

> Rewritten 2026-06-27 after competitive research. The wedge shifted: sloop owns **portable context
> + cross-repo fleet**, and **cedes in-repo orchestration** to mature tools (see Landscape below).

## North star

Sloop is the **local-first layer that makes your project context portable across AI CLIs and gives
you a cross-repo view of all your agents.** It is *not* another in-repo multi-agent orchestrator —
that lane is well served (ntm, Claude Squad). Sloop sits **above** those tools and complements them:

1. **Context, portable** — `AGENTS.md` is canonical; tools read it natively or via a thin pointer;
   `.sloop/skills` is symlinked into each tool. One source, every tool, no duplication. _(shipped)_
2. **Fleet, cross-repo** — see every running AI session **across all your repos** from anywhere,
   glance at what each is doing, jump fast. The cross-repo span is the edge (orchestrators are
   single-project). _(prototype shipped)_

**Hard rules:** never violate the AI provider (use its own hooks or non-invasive local signals,
never intercept/inject); stay a single lightweight CGO-free Go binary, no daemon, no bundled LLM.
**Provider-aware by construction:** all per-provider knowledge lives in the adapter manifest
(`docs/ADAPTERS.md`); features are manifest-driven, never hardcode a tool. User config is unified
(`docs/CONFIG.md`), not per-provider.

## Landscape (why the wedge is here)

- **ntm** (Go, ~40 cmds, conflict detection, TUI dashboard, REST API) and **Claude Squad** (Go,
  ~7.9k★, git-worktree isolation, TUI) already own **in-repo** multi-agent orchestration. Don't
  reinvent them.
- **Neither** does **context-file sync** (AGENTS.md/skills) or **cross-repo/multi-workspace** — that
  is sloop's open ground.
- `sloop run --split` overlaps them and is less mature (no worktree isolation) → kept as a minor
  convenience, **not** an investment area.

---

## Done (shipped)

- **Context delivery (Model B):** canonical `AGENTS.md`; pointer files create-if-missing;
  relative skills symlink (move-safe, self-heal, copy fallback); `sync --all`, `sync --repair`,
  `status`; `run … -- <args>`; README aligned.
- **`sloop init --scan`:** heuristic, no-LLM codebase scan → pre-filled `AGENTS.md` (language,
  build/test/lint commands with Makefile precedence, layout, README seed, Conventions placeholder).
- **Skills management:** `sloop skills new` (scaffold + auto-link into tools) and `sloop skills add`
  (import from a URL / GitHub blob). Command renamed `skill` → `skills` (aliases kept).
- **Cross-repo fleet prototype:** `sloop ps` (running sessions across workspaces) + glance (last
  output line, non-invasive) + `ps <#>` jump; `run --split` (panes, minor convenience).
- **Micro-TUI for `ps`/`ls`:** zero-dependency, colored, ANSI-raw interactive menu (arrow-key
  select / jump) — no external TUI lib, single binary preserved (`internal/tui`).
- **Precise-ish status + `sloop send`:** `ps` classifies each session from its own pane
  (non-invasive) into waiting / working / idle and floats "waiting on you" to the top;
  `sloop send <#|session|workspace> "msg"` injects a prompt via `tmux send-keys` without
  attaching. Provider hooks (Claude `Stop`/`Notification`) remain a later precision upgrade.

## Now — validating

Dogfood (`docs/USAGE.md`). The question: does `sloop ps` across your repos let you triage "which
agent needs me" in a way single-project tools (ntm/Squad) and raw tmux don't? That validates the
cross-repo wedge specifically — not generic orchestration.

---

## Next actions (post-dogfood, prioritized for the wedge)

1. **Fleet intelligence** —
   - _(shipped, slice 1)_ `ps --waiting` filter; `ps --watch [-n]` live monitor that bells +
     optional `--notify` (desktop) when an agent **newly** waits; concurrent pane capture for speed.
   - _(shipped, slice 2)_ **Status precision via provider hooks** — `sloop hooks install` adds
     Claude's own `UserPromptSubmit`/`Notification`/`Stop` hooks (each calling `sloop hook
     <state>`, persisted under `~/.sloop/state`); `ps` prefers a fresh marker over the
     heuristic. Provider-respecting (Claude's own hook mechanism), 15-min marker TTL, hardened
     with a per-capture timeout so a stuck pane can't hang the fleet view.
   - _(shipped, slice 3)_ **Registry-aware `ps` + multi-provider hook awareness** — `ps --all`
     adds registered-but-idle workspaces (with repo path) for the full cross-repo board; `hooks`
     is now a provider registry (`hooks list`/`print <tool>`) covering claude/gemini/cursor/
     copilot/codex — each tool's event→`sloop hook <state>` mapping is captured (Claude
     auto-installs; others are print-and-paste pending a verified installer per tool).
   - _(shipped)_ **Provider-aware consolidation (0.0.1 prep)** — hooks moved into the adapter
     manifest (single source); `hooks.go` is manifest-driven; **Gemini auto-installs** too
     (`settings-json` strategy, same shape as Claude). `sloop tools` is a capability matrix
     (context/skills/hooks). Empty `.sloop/profiles/` removed. Config carries `version: 1`.
     Docs: `docs/ADAPTERS.md` (contract), `docs/CONFIG.md` (layering), AGENTS.md rule.
   - _(next)_ **More auto-installers** — `hooks install cursor|copilot` (different JSON shapes) and a
     Codex `notify`-payload mode for `sloop hook` (TOML; no parser per no-new-modules). Group `ps`
     output by workspace; show repo path on running rows too.
2. **Skills lockfile → registry** ⭐ — on-thesis (context portability across tools *and* sources;
   ntm/Squad don't do skills distribution). Path: (a) shipped `skills add <url|github>`;
   (b) a `.sloop/skills.lock` recording each imported skill + source so `skills update` re-fetches
   and the team gets reproducible skills; (c) later, a registry (`skills search`/`add <name>`)
   resolving from skills.sh or a curated index. _(skills.sh's API/format needs investigating first.)_
3. **Cross-repo `ps`/`ls` registry-awareness** — now that the Micro-TUI shipped, make the list show
   *known* workspaces (from the registry), not only live tmux sessions; group by workspace, show repo
   path, sort by "needs-attention" (pairs with #1's precise status).
4. **Context-portability depth** — confirm/extend per-tool delivery (native vs pointer) for more
   tools; skills authored once → everywhere; keep AGENTS.md the single canonical source.
5. **`init --scan` LLM enrichment** — reuse a minimal LLM client to turn the heuristic scaffold into
   prose (Phase 5 Step C); only when a minimal client exists.
6. **Complementarity** — document/position sloop as working *alongside* ntm/Claude Squad (context +
   cross-repo) rather than competing; explore a light integration if it helps.

---

## Architecture (shipped)

- **`internal/tmux` package** — extracted all tmux/fleet code out of `internal/runner` (which stays
  the pure `Runner`/`Spec` launch abstraction); de-stuttered API (`tmux.Available`, `tmux.Session`,
  `tmux.ClassifyStatus`, `tmux.Runner`…). `Notify` folded into `commands`. Windows backend is a future
  sibling behind the same seam — no stub files (lightweight, no clutter).
- **SQLite durability** — global `sloop.db` opens with WAL + `busy_timeout` + `foreign_keys`, and a
  minimal `PRAGMA user_version` migration runner (append-one-string; no framework). Safe for
  concurrent cross-repo writes.

## Windows support (shipped, needs verification)

- **Multiplexer-agnostic backend** — `tmux.Bin()` auto-detects `tmux` then **psmux** (native Windows,
  tmux-CLI-compatible — same `new-session/list-sessions/capture-pane/send-keys/split-window/…`), with
  `SLOOP_MUX` override. All `exec.Command("tmux")` route through `Bin()`. macOS/Linux unchanged.
  **Lightweight win:** no WSL, no ConPTY reimplementation — psmux speaks tmux's CLI.
- **Windows desktop notifications** — `osNotify` adds a PowerShell balloon (best-effort).
- ⚠️ The psmux path is wired but **not yet verified on a real Windows machine** — needs a dogfood pass
  for exact flag/format-variable compatibility (`list-sessions -F`, `capture-pane -p`, etc.).

## Later / parked

- **`sloop init --scan` LLM enrichment** + **AI `sloop doctor`** — when reached, add a **minimal**
  `Complete(prompt)` client inside the first feature, not a standalone "LLM foundation".
- **Windows multiplexer** (`wt.exe`/`psmux`) — only if Windows users matter.
- **2nd-brain / RAG (pure-Go)** — embeddings as SQLite BLOBs, cosine in Go (no cgo); Obsidian bridge.
- **Markdown rendering** (`glow`/`glamour`), lifecycle hooks (`.sloop/hooks/*`).

## Ceded to ntm / Claude Squad (do NOT build — they own it)

- In-repo worktree isolation, TUI orchestration dashboard, file-conflict detection, broadcast-to-all,
  popup-HUD-as-orchestrator, pipelines/work-graphs/checkpoints. Sloop's launch story is **cross-repo
  `run -w`**, not in-repo swarm management.

## Dropped (with rationale)

- **Two-way sync engine** (`sync pull`/`diff`/`undo`, markers) — superseded by Model B; safe remainder
  shipped as `sync --repair`.
- **Standalone LLM provider "foundation"** — over-engineering ahead of any consumer.
- **Daemon / gRPC / socket IPC** — contradicted lightweight/local-first; removed at MVP.
