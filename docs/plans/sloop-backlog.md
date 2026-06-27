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

## Now — validating

Dogfood (`docs/USAGE.md`). The question: does `sloop ps` across your repos let you triage "which
agent needs me" in a way single-project tools (ntm/Squad) and raw tmux don't? That validates the
cross-repo wedge specifically — not generic orchestration.

---

## Next actions (post-dogfood, prioritized for the wedge)

1. **Cross-repo `ps` polish** ⭐ — registry-aware (show known workspaces, not only live tmux), group
   by workspace, show repo path, sort by "needs-attention". Make the cross-repo lens genuinely better
   than `tmux list-sessions`.
2. **Context-portability depth** — confirm/extend per-tool delivery (native vs pointer) for more
   tools; skills authored once → everywhere; keep AGENTS.md the single canonical source.
3. **Skills lockfile → registry** ⭐ — on-thesis (context portability across tools *and* sources;
   ntm/Squad don't do skills distribution). Path: (a) shipped `skills add <url|github>`;
   (b) a `.sloop/skills.lock` recording each imported skill + source so `skills update` re-fetches
   and the team gets reproducible skills; (c) later, a registry (`skills search`/`add <name>`)
   resolving from skills.sh or a curated index. _(skills.sh's API/format needs investigating first.)_
3. **`init --scan` LLM enrichment** — reuse a minimal LLM client to turn the heuristic scaffold into
   prose (Phase 5 Step C); only when a minimal client exists.
4. **Complementarity** — document/position sloop as working *alongside* ntm/Claude Squad (context +
   cross-repo) rather than competing; explore a light integration if it helps.
5. **Precise agent status & Remote messaging (`sloop send`)** ⭐ — Depend on provider hooks (e.g. Claude `Stop`/`Notification`) or shell markers to accurately detect if an agent is waiting. Once status is precise, introduce `sloop send <session> "msg"` (via `tmux send-keys`) to inject prompts without attaching. Overcomes the CLI limitation of lacking a direct chat box.

---

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
