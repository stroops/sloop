# `sloop run` — launching an agent (CLI · model · effort)

`sloop run` is the heart of sloop: it takes a short, friendly target and launches the right AI CLI,
with the right model and reasoning effort, in a managed session. This doc is the design contract —
the model, the resolution rules, the manifest schema, and the staged plan — so the upgrade is settled
before it's built.

> Command reference (today's behaviour): [USAGE.md](../guide/USAGE.md). Provider contract: [ADAPTERS.md](../reference/ADAPTERS.md).

## Two jobs, one clean seam

`run` does exactly two things, and keeping them separate is the design:

1. **Resolve → launch spec.** Turn the target + flags into `{binary, args}` — *what* to run.
2. **Provision → launch.** Create the tmux session/window and run the spec inside it — *where* to run.

Job 2 already exists and is stable (`runner.Runner`/`tmux.Runner`). The upgrade is **entirely in job
1**: today it only resolves a *tool*; it should resolve a tool **and** a model **and** an effort. No
change to how sessions are provisioned.

## Three concepts the target conflates

A single token like `claude`, `opus`, or `agent` can mean different things. Name them:

| Concept | Examples | Notes |
|---|---|---|
| **CLI / tool** | claude, cursor (`agent`), codex, agy | the binary sloop launches — today's "provider" |
| **Model** | opus, sonnet, gpt-5 | has a fixed **vendor** |
| **Effort** | low / medium / high | each CLI expresses reasoning differently |

Plus **vendor** (Anthropic, OpenAI, Google) — who makes the model. A model's vendor is unambiguous; a
model can be run by *several* CLIs.

## Resolving "is opus Claude or Cursor?" — via vendor

`model → CLI` is many-to-one (Cursor can also run opus), but **`model → vendor` is one-to-one**. So:

- **`sloop run opus`** → vendor = Anthropic → that vendor's default CLI (`claude`) → `claude --model opus`.
- **`sloop run cursor -m opus`** → CLI is named explicitly → cursor runs opus its own way.

> Rule: a bare model resolves to its vendor's home CLI; naming a CLI explicitly overrides it.

## Resolution algorithm

Given positional token `T` and flags `-p/--provider`, `-m/--model`, `-e/--effort`:

```
CLI    = --provider
       | T if T is a tool key OR a known detect/launch binary   (e.g. "agent" → cursor)
       | the home CLI of (model's vendor)
       | project default_tool
model  = --model
       | T if T is a known model alias (and wasn't used as the CLI)
       | (none → the CLI's own default)
effort = --effort | (none)

args   = [run.model_flag, model]            (if model set and the CLI supports it)
       + [run.effort_flag, effortToken]     (if effort set and the CLI supports it)
       + passthrough (everything after `--`)
```

Precedence when `T` is ambiguous: **tool key → binary alias → model alias**. Unknown `T` → a clear
error (`unknown tool or model "X"`), never a silent guess.

### Worked examples

| Command | CLI | Args |
|---|---|---|
| `sloop run claude` | claude | _(default model)_ |
| `sloop run agent` | cursor | _(binary alias of cursor)_ |
| `sloop run opus` | claude | `--model opus` _(vendor → home CLI)_ |
| `sloop run cursor -m opus` | cursor | cursor's model flag `opus` |
| `sloop run claude -m sonnet -e high` | claude | `--model sonnet` + effort token |
| `sloop run agy -m opus` | agy | **error**: agy has no `run.model_flag` |

## Where the knowledge lives

Per the manifest rule, per-CLI knowledge goes in the manifest; only the small, stable model→vendor map
is shared.

**Manifest `run.*` (per CLI):**
```yaml
launch: claude
run:
  vendor: anthropic          # the model vendor this CLI natively serves
  default_for: [anthropic]   # this CLI is the canonical launcher for these vendors (bare-model)
  model_flag: "--model"      # how to pass a model; "" / omitted = no model selection
  effort_flag: ""            # how to pass reasoning effort; "" = unsupported
  effort_values:             # map sloop's low|medium|high → this CLI's own token
    low: ""
    medium: ""
    high: ""
  models: [opus, sonnet, haiku, fable]   # model aliases this CLI serves (for completion + Phase 2;
                                         # full API strings like claude-opus-4-8 still work via forward)
```

**No separate model catalog file.** Model aliases live in the manifest's `run.models`, so the
alias→CLI map is just a reverse index over manifests — one mechanism, zero new files, embedded in the
binary and overridable the same way as any manifest (`~/.sloop/adapters/<tool>.yaml`). A bare
`sloop run opus` finds the manifest serving `opus`; if several do, `default_for` picks the home CLI.
The alias's vendor is implied by its home CLI's `run.vendor` — no global `models.yaml` to maintain.

**What sloop deliberately does *not* do:** keep a full, churning model catalog (ids, pricing, context
windows), nor **validate** the model — model selection is the CLI's own step; sloop only offers a
nice-to-have shortcut. It learns just enough (a small curated alias list) to pick the CLI and power
completion, then **forwards the model string to the CLI** (alias *or* full API id), letting the CLI
accept or reject it. Light, and resilient to new models.

**Why not persist models in SQLite?** The alias set is small, shared, and ships with the binary — it's
*reference* data (manifest), not *runtime* state. SQLite holds accumulated per-machine data (sessions,
fleet), not a model list that would need seeding/migrating per release. Dynamically discovering a
tool's models — via the tool's *own* `list`-style command (provider-respecting, never the vendor API),
cached with a TTL — is a possible later nicety for CLIs with large/volatile model sets. It's **opt-in
and parked**; curated aliases cover the common case at zero runtime cost.

> **User preference vs. reference knowledge.** The `run.*`/`models` above is *provider reference
> knowledge* (manifest). A user's own default — "this repo defaults to sonnet at high effort" — is
> *user config* and belongs in `.sloop/config.yaml` (Phase 3), never mixed into the catalog.

## What we borrow (and don't) from aider & the CLIs

- **aider** — model-first UX, provider inferred from the model name, shortcuts like `--opus`. We adopt
  **model-first + aliases**. We *don't* adopt its litellm-scale model registry: aider calls APIs;
  sloop launches a CLI and lets *it* own the model list.
- **The AI CLIs themselves** — each has its own `--model`/effort flag and is mostly single-vendor. We
  adopt **forward-to-CLI** and keep every flag name in the manifest, never hardcoded.

## Staged plan

- **Phase 1 — explicit flags (deterministic).** _(shipped)_ `-p/--provider`, `-m/--model`,
  `-e/--effort` forwarded via manifest `run.*`; plus binary-alias resolution (`run agent` ==
  `run cursor`). Zero new files, zero new dependencies — just manifest fields (embedded) and flag
  parsing (`planLaunch`/`buildRunArgs`).
- **Phase 2 — smart positional.** _(shipped)_ Classifies `T` as tool | binary | model alias by
  reverse-indexing manifest `run.models` (`modelHomeTool`), so `sloop run opus` → `claude --model opus`.
- **Initial task.** _(shipped)_ `-t/--task "…"` seeds an **interactive** session already working on
  the task (so it lives in the fleet), delivered per `run.prompt` (positional for claude/cursor —
  `claude "task"` / `agent "task"`; their `-p` is the non-interactive print mode, which sloop does not
  use). A CLI without `run.prompt` errors clearly. (Headless one-shot is parked — it doesn't live in
  the fleet, so it adds little over calling the CLI directly.)
- **Phase 3 — profiles & session identity.** User aliases (`sloop run myopus`), and session naming that
  includes the model so two Claude sessions (opus + sonnet) can coexist in one repo (today the session
  is `<workspace>__<tool>`, which would collide).

## Open questions (feedback before/while building)

- Effort vocabulary: is `low|medium|high` enough, or do some CLIs need a numeric/thinking-budget form?
- Session identity with model (Phase 3): always include the model in the name, or only when asked?
- User default model/effort (Phase 3): the shape of the `.sloop/config.yaml` preference (global vs
  per-tool).

## Contributing

Teaching sloop to launch a tool with a model/effort is **manifest-only**: add a `run:` block to the
tool's adapter (see [ADAPTERS.md](../reference/ADAPTERS.md)) — `model_flag`, `effort_flag`/`effort_values`,
`vendor`, `default_for`, `models`. No Go changes unless the CLI needs a brand-new launch mechanism.
