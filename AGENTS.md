# AGENTS.md

## Provider-aware architecture (read before adding provider behavior)

Sloop's reason to exist is **multi-AI-provider** support, so the codebase is provider-aware by
design: **all per-provider knowledge lives in the adapter manifest** (`internal/adapter/builtin/
<tool>.yaml`, user-overridable in `~/.sloop/adapters/`), and every provider-aware feature reads it.

**The rule: never hardcode a tool name in a feature. Add capability to the manifest instead.**
Detect, launch, context delivery, skills, hooks, and completion are all manifest-driven. To support a
new CLI, drop in one `<tool>.yaml` — no Go changes unless it needs a brand-new mechanism.

- Provider contract + manifest schema: `docs/ADAPTERS.md`
- Config layering (unified user config vs per-provider manifests): `docs/CONFIG.md`
- Runtime capability matrix: `sloop tools`

User config is **unified** (one local `.sloop/config.yaml` + one global `~/.sloop/config.yaml` +
the global DB), never split per provider.
