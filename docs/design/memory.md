# Memory / vault — the 2nd-brain bridge (DRAFT design)

> **Status: skeleton for discussion.** Nothing here is built or decided. This captures *what we want
> to do* and the constraints it must respect, so we can review and argue the shape before writing
> code. The delivery mechanism (a read-only `SessionStart` context-inject hook) is sketched in
> [hooks.md §Context-inject hooks](hooks.md); this doc owns the memory model itself.

## The problem

Portable context today is **static and write-once**: `init`/`sync` write `AGENTS.md` + per-tool
pointers, and that's it. Two gaps follow:

1. **Nothing is learned.** A decision made or a gotcha hit in session A is gone by session B. The user
   re-explains the same things to the same (or a different) tool.
2. **Nothing is shared across the fleet.** sloop's wedge is portable context + cross-repo fleet, but
   "context" is currently just a file the user maintains by hand — not something the fleet accrues.

The goal: a **2nd brain** that the workspace accrues over time and replays into any tool, any session,
any repo — without sloop becoming the model.

## Goals

- **Capture** durable facts/decisions/gotchas from real sessions (or explicit user input) into a local
  store.
- **Replay** the relevant slice back into a fresh session automatically, via the provider's own
  `SessionStart` mechanism (no rewriting prompts, no editing the tool's `CLAUDE.md`).
- **Portable** across the five+ CLIs and across repos — same store, any tool reads it.
- **Provider-respecting and private by construction** — same posture as status hooks (read-only,
  outbound-only, local; see hooks.md privacy note).

## Non-goals (bright lines)

- **No bundled LLM / RAG / embeddings / vector DB.** sloop is plumbing. "Relevance" is mechanical
  (recency, active workspace, simple matching), and any real semantic retrieval is delegated to the
  agent — *it* is the LLM. The moment we'd reach for embeddings, we stop and hand the job to the tool.
- **No silent capture of session content.** Writing to memory is an explicit, visible act (see the
  write-path question) — never a hook that quietly scrapes prompts/responses.
- **Not a knowledge base / wiki / sync service.** The vault is local notes the fleet replays, not a
  hosted product.

## Shape (two paths, decided separately)

Memory splits into a **read path** (replay) and a **write path** (capture). They're independent
decisions; we can ship the read path against a hand-written vault long before automating capture.

### Read path — replay (closest to ready)

```text
SessionStart → `sloop context emit` → selects relevant vault slice → provider injects it as context
```

- Read-only, outbound-only. Emits content sloop already owns under `.sloop/`. No session reading.
- Selection is plumbing: recency + active workspace + simple matching, under a token budget.
- Falls back to file-pointer delivery for tools that don't expose `SessionStart`.

### Write path — capture (more open)

How does a fact *get into* the vault? Candidates, not yet chosen:

- **Explicit** — `sloop remember "<fact>"` (and/or a `/remember`-style provider command). Highest
  trust, zero magic, user-curated. Likely the v1.
- **Prompted capture** — a `SessionEnd`/`Stop` hook that *asks* the user "save anything from this
  session?" rather than auto-scraping. Keeps the human in the loop.
- **Agent-authored** — the tool itself writes a note via a skill/command when it learns something.
  Delegates the "what's worth remembering" judgment to the model (consistent with the no-LLM line).

We deliberately **avoid** an auto-`Stop`-hook that reads and stores session content silently — that
would cross the privacy line we drew for status hooks.

## Storage model (sketch)

- Lives under `.sloop/vault/` (already scaffolded by `init`, already in `.sloop/.gitignore` as
  personal/not-shared by default).
- Plain files (Markdown, one fact per file?) so it's greppable, diffable, and editable by hand — and
  so the agent can read/write it with its normal file tools. Mirrors how *this* assistant's own
  `MEMORY.md` index + per-fact files work; that pattern is a candidate to copy outright.
- **Open:** per-fact files + an index vs. a single file; frontmatter schema (type, scope, created);
  personal (gitignored) vs. team-shared (committed) tiers; per-workspace vs. a global cross-repo vault
  (the cross-repo wedge probably wants *both*: a global brain + a per-workspace overlay).

## Open questions (the things to argue in review)

- **Relevance without a model.** What's the actual selection rule for "what to inject at SessionStart"?
  Whole vault (simple, but blows the token budget fast) vs. a recency/scope slice vs. tag-matching on
  the workspace. Where's the budget cap?
- **Write trigger.** Explicit `sloop remember` first, or invest in prompted capture early? What's the
  smallest thing that's genuinely useful?
- **Scope & sharing.** Per-workspace vs. global brain; personal (gitignored) vs. committed/team. How do
  the two layers compose at injection time?
- **Cross-repo replay.** The wedge says context should follow you across repos — does a global vault
  inject into *every* workspace, or only on demand? How is "this fact is about repo X" expressed?
- **Provider coverage.** Which CLIs expose `SessionStart` (and an injectable payload)? Manifest field +
  fallback for those that don't.
- **De-dup / decay.** How do stale or contradicted facts get pruned? (Our own memory rules — "update
  the existing file, delete what's wrong" — are a candidate policy.)
- **Interop.** Should sloop's vault read/emit alongside a tool's *native* memory (e.g. a provider's own
  memory feature) rather than compete with it?

## Phasing (proposal)

1. **Read path on a hand-written vault.** `sloop context emit` + one provider's `SessionStart` wiring;
   user maintains `.sloop/vault/` by hand. Proves the injection channel end to end.
2. **Explicit capture.** `sloop remember` writes a fact; emit picks it up next session.
3. **Selection + scope.** Recency/scope slicing, token budget, per-workspace vs. global layering.
4. **Cross-repo brain.** Global vault that replays into any workspace; team-shared tier.

(Each phase is independently shippable and independently reviewable. We are not committing to past
phase 1 until phase 1 earns it.)

## Related

- [hooks.md](hooks.md) — the three hook uses; the `SessionStart` context-inject channel this doc rides
  on, and the privacy posture it inherits.
- Product direction: portable context + cross-repo fleet is the wedge; keep LLM out of the core.
