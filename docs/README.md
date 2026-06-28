# Sloop docs

Documentation is organized by **purpose**, so it stays navigable as it grows:

| Folder | Purpose | Audience |
|---|---|---|
| **[guide/](guide/)** | how-to, hands-on usage | users |
| **[reference/](reference/)** | how the system *is* — stable contracts & internals | users + contributors |
| **[design/](design/)** | how a feature *should be* — proposals, evolving | contributors |
| `local/` | private working notes (gitignored) | maintainers |

## guide/
- [USAGE.md](guide/USAGE.md) — every command, with examples

## reference/
- [ARCHITECTURE.md](reference/ARCHITECTURE.md) — packages, data flow, internals
- [ADAPTERS.md](reference/ADAPTERS.md) — the provider-aware adapter manifest contract
- [CONFIG.md](reference/CONFIG.md) — the config layers (local / global / built-in)

## design/
- [run.md](design/run.md) — `sloop run`: CLI · model · effort resolution
- [hooks.md](design/hooks.md) — status hooks today, workflow hooks (v0.2.0)
- [skills.md](design/skills.md) — skills model, lockfile, registry roadmap

> Project direction lives in [../ROADMAP.md](../ROADMAP.md); shipped changes in
> [../CHANGELOG.md](../CHANGELOG.md).

**Adding a doc:** put it in the folder matching its purpose (a new feature design → `design/`; a new
stable contract → `reference/`) and link it here.
