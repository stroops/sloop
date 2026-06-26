# Sloop Backlog

> Rewritten 2026-06-26 to match the validated direction. The old phase list (full-copy sync,
> two-way engine, grand LLM foundation) was retired — see **Dropped** at the bottom for what and why.

## North star

Sloop is a **local-first, AI-aware orchestration layer over AI coding CLIs**. It does two things:

1. **Context, once** — `AGENTS.md` is canonical; tools read it natively or via a thin pointer;
   `.sloop/skills` is symlinked into each tool. (Largely *done* — and commoditizing as tools adopt
   `AGENTS.md` natively, so not where the moat is.)
2. **Fleet, aware** — manage the many concurrent AI sessions you run: see what each agent is doing,
   jump fast, run them side-by-side. **This is the moat** — it needs local context tmux can't have,
   and it's the founding pain ("too many windows").

**Hard rule:** AI-awareness must **never violate the AI provider** — use the provider's own
hooks/notifications, or non-invasive observation of *your own* terminal (`tmux capture-pane`,
activity, pane state). Never intercept its API, inject into its process, or read private internals.

---

## Done (shipped)

**Context delivery (Model B / sync v2 + hardening)**
- `AGENTS.md` canonical; pointer files (`CLAUDE.md`/`GEMINI.md`) create-if-missing, never overwrite;
  native tools read `AGENTS.md` directly.
- `.sloop/skills` **relative** symlink into each tool's skills dir (survives repo moves), copy
  fallback, self-heal, `broken` state.
- `sloop sync --all`, `sloop sync --repair` (non-destructive backup), `sloop status` delivery line.
- `sloop run … -- <args>` passthrough. README aligned to Model B.

**Orchestration prototype (the moat — being validated)**
- `sloop ps` — live fleet of running AI sessions across workspaces.
- `sloop ps <#>` — semantic jump (switch-client when inside tmux).
- `sloop run --split <tools…>` — side-by-side tmux panes on one repo.
- **glance** — each session's last terminal line in `ps` (non-invasive awareness).

---

## Now — validating

Dogfood the orchestration prototype (`docs/USAGE.md`). The decision it answers: does
`ps` + glance + `--split` triage "which agent needs me" across repos faster than raw tmux? If yes,
double down (Next §1–3). If not, learn cheaply and reconsider the moat.

---

## Next actions (post-dogfood, prioritized)

1. **Precise agent status via the provider's own hooks** ⭐ — e.g. Claude Code `Stop`/`Notification`
   hooks write a status crumb sloop reads, so `ps` shows **⏸ waiting-for-you / ● working / ✓ done**
   instead of guessing from the glance. The cleanest, fully-sanctioned signal; glance stays the
   universal fallback for tools without hooks.
2. **Popup HUD** — a tmux `display-popup` keybinding (e.g. `Ctrl-b S`) opens `sloop ps` over the
   current pane to jump, then closes. Removes the "return to sloop" friction; makes sloop an overlay,
   not a competing window layer.
3. **`sloop ps --watch`** — auto-refreshing live dashboard of the fleet.
4. **`sloop init --scan`** — heuristic, no-LLM codebase scan → pre-filled `AGENTS.md`. Spec + plan
   ready: `docs/superpowers/{specs,plans}/2026-06-26-sloop-init-scan*`. Improves onboarding
   (codebase-first reality).

---

## Later / parked (good ideas, not now)

- **Windows multiplexer** — map orchestration onto Windows Terminal (`wt.exe`) / `psmux` so `ps` and
  `--split` work off tmux. Required only if Windows users matter; the runner already abstracts launch.
- **AI `sloop doctor`** — LLM reviews `AGENTS.md` + `.sloop/skills` for gaps/redundancy/conflicts.
  When built, add a **minimal** `Complete(prompt)` client inside it (one provider, key/model config) —
  **not** a standalone multi-provider "LLM foundation".
- **Auto-improvement (meta-agent)** — detect a manual fix to AI-generated code → generate a
  "lesson learned" skill. Fuzziest item; needs change-detection; depends on the hooks work above.
- **Lifecycle hooks** — `.sloop/hooks/{pre-run,post-sync}.sh` triggered by sloop commands.
- **2nd-brain / RAG (pure-Go)** — keep `modernc.org/sqlite`; store embeddings as BLOBs, cosine
  similarity in Go memory (no cgo, no `sqlite-vss`). Formalize in `docs/architecture/vector-rag.md`
  when started. External vault bridge (Obsidian) as a symlink/API.
- **Markdown rendering** — `sloop view <skill|context>` via `glow`/`bat` if present, or embed
  `charmbracelet/glamour`.

---

## Dropped (with rationale — don't resurrect without revisiting these)

- **Two-way sync engine (`sync pull` / `diff` / `undo`, managed-region markers)** — written for the
  old v1 full-copy model. Under Model B it's unnecessary: skills are symlinked (already two-way),
  `AGENTS.md` is canonical (nothing to reconcile). The only safe remainder shipped as
  `sync --repair`. The one theoretical residual (copy-fallback edits not flowing back, on
  symlink-incapable hosts) is a noted limitation, not a feature.
- **Standalone "LLM provider foundation"** (multi-provider routing, free-tier optimization, key
  abstraction, Ollama) — over-engineering ahead of any consumer. Build the client minimally inside
  the first feature that needs it; generalize only when a 2nd caller (RAG) appears.
- **Daemon / gRPC / socket IPC** — contradicted "lightweight, local-first"; removed at MVP.
