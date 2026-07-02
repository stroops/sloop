package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/fleetstate"
	"github.com/stroops/sloop/internal/workspace"
)

// The statusline feed is the second half of provider-aware status: tools like
// Claude and Antigravity pipe a JSON payload (model, context usage, state) to
// a user-configured statusline command on every render. `sloop statusline
// install` registers `sloop statusline feed <tool>` as that command; the feed
// records what the tool reports into the fleet marker, then either chains to
// the statusline command the user already had (their in-TUI bar is untouched)
// or renders a simple default line. Which fields live where in the payload is
// declared per-tool in the adapter manifest (statusline.payload), never here.

// payloadAt walks a dotted path ("model.display_name") through a decoded JSON
// document. Missing or non-object intermediate nodes yield ok=false.
func payloadAt(doc map[string]any, path string) (any, bool) {
	if doc == nil || path == "" {
		return nil, false
	}
	var cur any = doc
	for part := range strings.SplitSeq(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[part]; !ok {
			return nil, false
		}
	}
	return cur, true
}

// payloadStr returns the string at a dotted path, "" when absent or not a string.
func payloadStr(doc map[string]any, path string) string {
	v, ok := payloadAt(doc, path)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// payloadNum returns the number at a dotted path.
func payloadNum(doc map[string]any, path string) (float64, bool) {
	v, ok := payloadAt(doc, path)
	if !ok {
		return 0, false
	}
	n, ok := v.(float64) // encoding/json decodes all JSON numbers to float64
	return n, ok
}

// extractStatusInfo pulls the model name and context percentage out of a
// statusline payload using the manifest's field mapping. The percentage comes
// either ready-made (context_pct) or computed from summed token paths against
// the window size — whichever the tool sends. Unknown fields yield zero values,
// never errors: the feed must degrade, not fail.
func extractStatusInfo(doc map[string]any, p adapter.StatusLinePayload) (model string, ctxPct int) {
	model = payloadStr(doc, p.Model)
	if n, ok := payloadNum(doc, p.ContextPct); ok {
		return model, clampPct(n)
	}
	if len(p.ContextUsed) == 0 || p.ContextSize == "" {
		return model, 0
	}
	size, ok := payloadNum(doc, p.ContextSize)
	if !ok || size <= 0 {
		return model, 0
	}
	used, any := 0.0, false
	for _, path := range p.ContextUsed {
		if n, ok := payloadNum(doc, path); ok {
			used += n
			any = true
		}
	}
	if !any {
		return model, 0
	}
	return model, clampPct(used / size * 100)
}

func clampPct(n float64) int {
	return int(math.Round(math.Min(100, math.Max(0, n))))
}

// extractStatusState maps the tool's own payload state (if declared) to a
// sloop status via the manifest's states table; "" when unmapped.
func extractStatusState(doc map[string]any, spec adapter.StatusLineSpec) string {
	raw := payloadStr(doc, spec.Payload.State)
	if raw == "" {
		return ""
	}
	return spec.States[raw]
}

// defaultInlineStatus renders the line shown inside the tool's own TUI when
// the user had no statusline command of their own — sloop's freebie:
// `dir | model | ⎇ branch | ctx 45%`, ANSI-colored, fields omitted when unknown.
func defaultInlineStatus(doc map[string]any, p adapter.StatusLinePayload) string {
	const (
		dim  = "\033[90m"
		cyan = "\033[36m"
		blue = "\033[34m"
		yel  = "\033[33m"
		red  = "\033[31m"
		rst  = "\033[0m"
	)
	cwd := payloadStr(doc, p.Cwd)
	model, pct := extractStatusInfo(doc, p)
	ic := activeIcons()
	var segs []string
	if cwd != "" {
		segs = append(segs, blue+filepath.Base(cwd)+rst)
	}
	if model != "" {
		segs = append(segs, cyan+ic.Model+" "+model+rst)
	}
	if branch := gitBranch(cwd); branch != "" {
		segs = append(segs, yel+ic.Branch+" "+branch+rst)
	}
	if pct > 0 {
		c := dim
		switch classifyCtxPct(pct) {
		case ctxCrit:
			c = red
		case ctxWarn:
			c = yel
		}
		segs = append(segs, fmt.Sprintf("%s%s %s %d%%%s", c, ic.Ctx, contextBar(pct), pct, rst))
	}
	return joinWith(dim+" | "+rst, segs...)
}

var feedChain string

var statuslineFeedCmd = &cobra.Command{
	Use:    "feed <tool>",
	Short:  "Internal: record a tool's statusline payload (called by the tool itself)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Read the whole payload first: the marker write and the chained command
		// both need it, and draining stdin avoids a broken pipe on the tool's side.
		payload, _ := io.ReadAll(cmd.InOrStdin())

		// Everything below is best-effort: a statusline feed that errors or
		// exits non-zero would degrade the tool's own TUI, so we never do.
		var doc map[string]any
		_ = json.Unmarshal(payload, &doc)
		if m, err := manifestForTool(args[0]); err == nil && doc != nil {
			// The tool calls this on every statusline render — often many times a
			// minute with an unchanged payload — so only touch the marker file
			// when it actually has new information to record.
			if session := currentSession(); session != "" {
				curModel, curPct := fleetstate.Info(session)
				model, pct := extractStatusInfo(doc, m.StatusLine.Payload)
				if (model != "" && model != curModel) || (pct > 0 && pct != curPct) {
					_ = fleetstate.WriteInfo(session, model, pct)
				}
				if st := extractStatusState(doc, m.StatusLine); st != "" {
					if cur, fresh := fleetstate.Read(session); !fresh || st != cur.Status {
						_ = fleetstate.Write(session, st)
					}
				}
			}
			if feedChain == "" {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), defaultInlineStatus(doc, m.StatusLine.Payload))
				return nil
			}
		}
		if feedChain != "" {
			chain := exec.Command("sh", "-c", feedChain)
			chain.Stdin = bytes.NewReader(payload)
			chain.Stdout = cmd.OutOrStdout()
			chain.Stderr = io.Discard
			_ = chain.Run()
		}
		return nil
	},
}

// shellQuote wraps s in single quotes for safe embedding in a sh -c command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// mergeStatuslineFeed points a settings.json statusLine at feedCmd, chaining
// to any command already there so it isn't lost. Returns the doc, the command
// now installed (existing or new), and whether the doc changed. Idempotent: a
// command already containing feedCmd is left alone.
func mergeStatuslineFeed(doc map[string]any, feedCmd string) (out map[string]any, installed string, changed bool) {
	sl, _ := doc["statusLine"].(map[string]any)
	existing, _ := sl["command"].(string)
	if strings.Contains(existing, feedCmd) {
		return doc, existing, false
	}
	installed = feedCmd
	if strings.TrimSpace(existing) != "" {
		installed += " --chain " + shellQuote(existing)
	}
	if sl == nil {
		sl = map[string]any{}
	}
	sl["type"] = "command"
	sl["command"] = installed
	doc["statusLine"] = sl
	return doc, installed, true
}

// installStatuslineFeed points a settings.json-style statusLine at the sloop
// feed, preserving any command the user already had by chaining to it
// (--chain). Idempotent: a command already containing the feed is left alone.
// Returns whether the file changed and the command now installed.
func installStatuslineFeed(path, feedCmd string) (bool, string, error) {
	var installed string
	changed, err := updateJSONFile(path, func(doc map[string]any) (map[string]any, bool) {
		var out map[string]any
		var ok bool
		out, installed, ok = mergeStatuslineFeed(doc, feedCmd)
		return out, ok
	})
	return changed, installed, err
}

// statuslineInstalledFor reports whether a tool's statusline feed is already
// wired into its config file (read-only; reuses the same idempotency check as
// install). Only meaningful for the settings-json strategy; false when the
// config is absent.
func statuslineInstalledFor(root, tool string, m adapter.Manifest) bool {
	if m.StatusLine.Config == "" || m.StatusLine.Install != "settings-json" {
		return false
	}
	path, err := resolveHookConfigPath(root, m.Account, m.StatusLine.Config)
	if err != nil {
		return false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc map[string]any
	if json.Unmarshal(b, &doc) != nil {
		return false
	}
	_, _, changed := mergeStatuslineFeed(doc, appName+" statusline feed "+tool)
	return !changed
}

var statuslineInstallCmd = &cobra.Command{
	Use:   "install [tool]",
	Short: "Feed a tool's own statusline into sloop (model + context in the status bar)",
	Long: `Register sloop as the tool's statusline command, so the tool itself reports
its model and context usage into the fleet view and the tmux status bar.

If a statusline command is already configured, sloop chains to it: the tool's
own display stays exactly as it was, sloop just reads the payload on the way.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tool := fallbackTool
		if len(args) == 1 {
			tool = args[0]
		}
		m, err := manifestForTool(tool)
		if err != nil {
			return err
		}
		if m.StatusLine.Config == "" || m.StatusLine.Install != "settings-json" {
			cmd.Printf("%s has no statusline mechanism sloop can hook into (yet).\n", m.Name)
			return nil
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		ws, err := workspace.Resolve(cwd)
		if err != nil {
			return err
		}
		path, err := resolveHookConfigPath(ws.Root, m.Account, m.StatusLine.Config)
		if err != nil {
			return err
		}
		changed, installed, err := installStatuslineFeed(path, appName+" statusline feed "+tool)
		if err != nil {
			return err
		}
		if !changed {
			cmd.Printf("sloop statusline feed already present in %s\n", path)
			return nil
		}
		cmd.Printf("installed statusline feed → %s\n", path)
		if strings.Contains(installed, "--chain") {
			cmd.Println("your existing statusline command is preserved (sloop chains to it).")
		}
		cmd.Printf("%s will now report its model and context usage to the fleet view.\n", m.Name)
		return nil
	},
}
