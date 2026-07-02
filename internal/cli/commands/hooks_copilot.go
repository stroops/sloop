package commands

import "github.com/stroops/sloop/internal/adapter"

// Copilot loads every *.json file under ~/.copilot/hooks/, so sloop owns a
// whole file (sloop.json) instead of merging into a shared one — no foreign
// hooks to preserve, and uninstall is "delete the file". Shape per
// https://docs.github.com/en/copilot/reference/hooks-reference:
// {version, hooks{event: [{type, command, matcher?}]}}. The `command` key is
// Copilot's cross-platform fallback, so there's no per-OS bash/powershell
// branching to do.

// copilotHooksDoc renders the full sloop-owned hooks document for a manifest's
// event mapping. States without an event are skipped, as everywhere else.
func copilotHooksDoc(h adapter.HooksSpec) map[string]any {
	hooks := map[string]any{}
	add := func(e adapter.EventSpec, state string) {
		if e.Event == "" {
			return
		}
		entry := map[string]any{"type": "command", "command": hookCommandFor(state)}
		if e.Matcher != "" {
			entry["matcher"] = e.Matcher
		}
		arr, _ := hooks[e.Event].([]any)
		hooks[e.Event] = append(arr, entry)
	}
	add(h.Events.Working, "working")
	add(h.Events.Waiting, "waiting")
	add(h.Events.Idle, "idle")
	return map[string]any{"version": 1, "hooks": hooks}
}

// installCopilotHooks writes the sloop-owned hooks file, reporting whether it
// changed. updateJSONFile keeps the read→compare→write skeleton (and the
// "never overwrite unparseable JSON" guarantee) shared with the other
// installers.
func installCopilotHooks(path string, h adapter.HooksSpec) (bool, error) {
	want := copilotHooksDoc(h)
	return updateJSONFile(path, func(doc map[string]any) (map[string]any, bool) {
		if jsonEqual(doc, want) {
			return doc, false
		}
		return want, true
	})
}
