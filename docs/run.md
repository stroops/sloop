# `sloop run` — launching an agent (CLI · model · effort)

`sloop run` is the heart of sloop: it takes a short, friendly target and launches the right AI CLI,
with the right model and reasoning effort, in a managed session. This doc is the design contract —
the model, the resolution rules, the manifest schema, and the staged plan — so the upgrade is settled
before it's built.

> Command reference (today's behaviour): [USAGE.md](USAGE.md). Provider contract: [ADAPTERS.md](ADAPTERS.md).

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
```

**Model catalog (embedded `models.yaml`, user-overridable at `~/.sloop/models.yaml`):**
```yaml
# alias → vendor (+ optional canonical id to forward instead of the alias)
opus:   { vendor: anthropic }
sonnet: { vendor: anthropic }
gpt-5:  { vendor: openai }
gemini-2.5-pro: { vendor: google }
```

**What sloop deliberately does *not* do:** maintain a full, churning model catalog (ids, pricing,
context windows). It learns just enough — alias → vendor (to pick the CLI) — and **forwards the model
string to the CLI**, letting the CLI validate it. Light, and resilient to new models.

## What we borrow (and don't) from aider & the CLIs

- **aider** — model-first UX, provider inferred from the model name, shortcuts like `--opus`. We adopt
  **model-first + aliases**. We *don't* adopt its litellm-scale model registry: aider calls APIs;
  sloop launches a CLI and lets *it* own the model list.
- **The AI CLIs themselves** — each has its own `--model`/effort flag and is mostly single-vendor. We
  adopt **forward-to-CLI** and keep every flag name in the manifest, never hardcoded.

## Staged plan

- **Phase 1 — explicit flags (deterministic).** `-p/--provider`, `-m/--model`, `-e/--effort` forwarded
  via manifest `run.*`; plus binary-alias resolution (`run agent` == `run cursor`). No inference, so no
  ambiguity — and it forces the `run.*` schema that Phase 2 builds on.
- **Phase 2 — smart positional.** Classify `T` as tool | model | alias using the vendor catalog, so
  `sloop run opus` / `sloop run sonnet` work.
- **Phase 3 — profiles & session identity.** User aliases (`sloop run myopus`), and session naming that
  includes the model so two Claude sessions (opus + sonnet) can coexist in one repo (today the session
  is `<workspace>__<tool>`, which would collide).

## Open questions (feedback before/while building)

- Effort vocabulary: is `low|medium|high` enough, or do some CLIs need a numeric/thinking-budget form?
- Catalog ownership: keep `models.yaml` tiny and curated, or let each manifest also declare the models
  it serves (and reverse-index)?
- Session identity with model (Phase 3): always include the model in the name, or only when asked?

## Contributing

Teaching sloop to launch a tool with a model/effort is **manifest-only**: add a `run:` block to the
tool's adapter (see [ADAPTERS.md](ADAPTERS.md)) — `model_flag`, `effort_flag`/`effort_values`,
`vendor`, `default_for`. No Go changes unless the CLI needs a brand-new launch mechanism.
