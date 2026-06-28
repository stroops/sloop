# Sloop Roadmap

Sloop is the **local-first layer that makes your project context portable across AI CLIs and gives
you a cross-repo view of all your agents.** It sits *above* your AI coding tools and complements them
— one canonical context, one fleet view, every tool.

This file is the public, contributor-facing roadmap. It links out to a design doc per pillar so a
contribution starts from a clear contract. For the shipped detail of each release, see
[CHANGELOG.md](CHANGELOG.md).

## Principles (what keeps sloop sloop)

- **Provider-respecting.** Never intercept or inject into an AI tool. Use the tool's *own* hook
  mechanism or non-invasive local signals only.
- **Provider-aware by construction.** All per-provider knowledge lives in one adapter manifest; no
  feature hardcodes a tool name. See [docs/reference/ADAPTERS.md](docs/reference/ADAPTERS.md).
- **One lightweight binary.** CGO-free Go, no daemon, no bundled LLM. Add capability when the first
  real feature needs it — never a standalone "foundation" ahead of a consumer.
- **Local-first & portable.** Author context/skills once; every tool and every repo sees them.

## Pillars

| Pillar | What it is | Design doc | Status |
|---|---|---|---|
| **Launch (`run`)** | one friendly target → the right CLI, model and effort, in a managed session | [docs/design/run.md](docs/design/run.md) | tool launch shipped; model/effort → next |
| **Portable context** | `AGENTS.md` canonical; pointer files + skills delivered to every tool | [docs/reference/ADAPTERS.md](docs/reference/ADAPTERS.md) | shipped (v0.1.0) |
| **Skills** | reusable prompts/workflows shared across tools *and* sources | [docs/design/skills.md](docs/design/skills.md) | shipping incrementally |
| **Cross-repo fleet** | every running agent across all repos; triage who needs you | [docs/guide/USAGE.md](docs/guide/USAGE.md) | shipped (v0.1.0) |
| **Hooks** | precise agent status today; a portable workflow-hook library next | [docs/design/hooks.md](docs/design/hooks.md) | status hooks shipped; workflow hooks → v0.2.0 |

## Now (v0.1.x — validating the wedge)

- Dogfood `sloop ps` across repos: does the cross-repo view triage "which agent needs me" better than
  single-project tools and raw tmux?
- **Skills:** lockfile (`.sloop/skills.lock`) + `skills update` for reproducible team skills. *(shipped)*
- **Hooks (status):** auto-installers per provider — claude, gemini, cursor done; copilot/codex need a
  matcher-aware model (see the hooks doc).

## Next (v0.2.0 — workflow hooks)

The big one. Today's hooks only report status *to sloop*. v0.2.0 turns hooks into a **portable
workflow-automation library**: pick a hook (format-on-edit, commit-policy, shell-guard, prompt-rule),
and sloop installs it into the right tool's own hook config — author once, run across tools and repos.
This is a large, security-sensitive surface, so it is **designed before it is built**. Full proposal,
categories, project/user levels, cross-tool mapping, and the trust model are in
**[docs/design/hooks.md](docs/design/hooks.md)**.

## Later / parked

- Skills registry (`skills search`/`add <name>` from a curated index) — after the lockfile proves out.
- `init --scan` LLM enrichment — only once a minimal LLM client exists for a real consumer.
- Windows multiplexer verification (psmux); 2nd-brain / RAG bridge.

## Contributing

The lowest-barrier, highest-leverage contributions are **data, not code**:

- **Add an AI tool:** drop one `internal/adapter/builtin/<tool>.yaml` — see
  [docs/reference/ADAPTERS.md](docs/reference/ADAPTERS.md). No Go changes unless the tool needs a brand-new mechanism.
- **Share a skill:** see [docs/design/skills.md](docs/design/skills.md).
- **Propose a workflow hook:** see the open questions in [docs/design/hooks.md](docs/design/hooks.md) — feedback on
  the model is wanted *before* the v0.2.0 build.
