package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/fleetstate"
	"github.com/stroops/sloop/internal/tmux"
)

// The status bar puts everything that matters on the LEFT, since tmux
// truncates status-right first on a narrow terminal: status, identity,
// model, context usage, and git branch, in that order. The RIGHT side only
// carries a rotating hint — genuinely ambient, safe to lose first. tmux's own
// window list (which would otherwise sit between them) is hidden; a sloop
// session is one window, so it only ever duplicated the identity already on
// the left. Both sides are rendered by sloop via tmux's #() every
// status-interval.

// statusSep is the dim separator between segments, so the bar reads as
// distinct fields instead of one run-on string.
const statusSep = " #[fg=colour238]│#[default] "

// The right side keeps only the rotating hint, dropped below this width —
// everything else lives on the left, where tmux truncates last, not first.
const minWidthHint = 110

// hintSlotSeconds is how long each rotating hint holds before advancing to
// the next one — long enough to read, short enough that the bar doesn't feel
// static.
const hintSlotSeconds = 20

// statusIcons is the bar's glyph set. Both renderers that draw icons — this
// file's tmux segments and statuslinefeed.go's ANSI default line — share it,
// so switching fonts is a one-line change instead of a find-and-replace.
type statusIcons struct {
	Waiting, Working, Idle string
	Model, Ctx, Branch     string
	RateLimit              string
}

// iconsUnicode is the default: plain Unicode that renders in any terminal,
// no font install required.
var iconsUnicode = statusIcons{
	Waiting: "◆", Working: "▸", Idle: "○",
	Model: "✧", Ctx: "▤", Branch: "⎇",
	RateLimit: "⏳",
}

// iconsNerd is the opt-in set for terminals with a patched Nerd Font
// installed (https://www.nerdfonts.com) — denser, more distinct glyphs.
var iconsNerd = statusIcons{
	Waiting: "󰔟", Working: "", Idle: "",
	Model: "", Ctx: "󱍏", Branch: "",
	RateLimit: "",
}

// activeIcons picks the glyph set: SLOOP_NERD_FONTS=1 (or true/yes/on) opts
// into iconsNerd; unset or anything else keeps the universally-safe default.
func activeIcons() statusIcons {
	switch strings.ToLower(os.Getenv("SLOOP_NERD_FONTS")) {
	case "1", "true", "yes", "on":
		return iconsNerd
	default:
		return iconsUnicode
	}
}

// tmuxStatusLabel renders an agent status with tmux's own color format
// (`#[fg=…]…#[default]`), kept short and clear for the status bar.
func tmuxStatusLabel(s tmux.AgentStatus) string {
	ic := activeIcons()
	switch s {
	case tmux.StatusWaiting:
		return "#[fg=yellow]" + ic.Waiting + " waiting#[default]"
	case tmux.StatusWorking:
		return "#[fg=cyan]" + ic.Working + " working#[default]"
	case tmux.StatusIdle:
		return "#[fg=green]" + ic.Idle + " idle#[default]"
	default:
		return ic.Idle
	}
}

// capturePane returns a lazy, memoized reader of a session's visible pane
// text, so the consumers within one render (status heuristic, model heuristic)
// share a single capture-pane subprocess — or skip it entirely when a fresh
// marker answers first.
func capturePane(session string) func() string {
	var text string
	var done bool
	return func() string {
		if !done {
			done = true
			if out, err := tmux.Output(tmux.BuildCaptureArgs(session)...); err == nil {
				text = string(out)
			}
		}
		return text
	}
}

// sessionStatusFrom resolves an agent status from an already-loaded marker: a
// fresh hook-written status wins, else the pane-text heuristic (capture is
// only invoked on that fallback).
func sessionStatusFrom(marker fleetstate.State, capture func() string, m adapter.Manifest) tmux.AgentStatus {
	if status, ok := marker.StatusFresh(); ok {
		return stateToStatus(status)
	}
	if text := capture(); text != "" {
		return tmux.ClassifyStatus(text, m)
	}
	return tmux.StatusUnknown
}

// sessionStatus is the one-shot form for callers with no marker in hand.
func sessionStatus(session string, manifests map[string]adapter.Manifest, tool string) tmux.AgentStatus {
	return sessionStatusFrom(fleetstate.Load(session), capturePane(session), manifests[tool])
}

// renderStatusline builds the legacy one-line status bar text (identity +
// status + fleet badge). Sessions created by older sloops still invoke this
// via their saved `#(sloop statusline <session>)` status-right.
func renderStatusline(session string) string {
	if session == "" {
		return ""
	}
	manifests, _ := adapter.Load()
	ws, tool, instance := splitSession(session, manifests)
	label := tool
	if instance != "" {
		label = tool + "·" + instance // distinguish a second agent of the same tool
	}
	st := sessionStatus(session, manifests, tool)
	return fmt.Sprintf("⚓ %s %s %s%s", ws, label, tmuxStatusLabel(st), renderFleetBadge(session))
}

// renderStatuslineLeft is the primary side: `⚓ ◆ waiting │ ws·tool │ model │
// ctx 45% │ ⎇ branch │ ⏳ 24% (45m)` — status, identity, and (when nothing
// else on screen already shows them) model/context/branch, plus rate-limit
// usage, which nothing else ever shows. It lives on the left because tmux
// truncates status-right first on a narrow terminal, and this is the content
// that must survive that. The window list tmux would normally draw in the
// middle is hidden (see tmux.SetStatusLine), so this and the hint on the
// right are the whole bar.
func renderStatuslineLeft(session string) string {
	if session == "" {
		return ""
	}
	manifests, _ := adapter.Load()
	ws, tool, instance := splitSession(session, manifests)
	label := tool
	if instance != "" {
		label = tool + "·" + instance // distinguish a second agent of the same tool
	}
	// One marker read and at most one pane capture serve the whole render.
	marker := fleetstate.Load(session)
	capture := capturePane(session)
	st := sessionStatusFrom(marker, capture, manifests[tool])

	dir, _ := paneInfo(session)
	model, ctxPct := marker.DisplayInfo()
	// Tools with no statusline feed can still show the model live via a pane
	// heuristic (manifest heuristics.model); the marker keeps the last match so
	// a redraw that briefly hides the footer doesn't blank the segment.
	if pattern := manifests[tool].Heuristics.Model; pattern != "" {
		if m := extractModel(capture(), pattern); m != "" && m != model {
			model = m
			_ = fleetstate.WriteInfo(session, m, 0)
		}
	}

	segs := []string{tmuxStatusLabel(st) + renderFleetBadge(session), ws + "·" + label}

	// sloop-only info: nothing else on screen ever shows it (a tool's own
	// footer, custom or sloop's freebie, has no rate-limit field), so it's
	// unconditional either way.
	segs = append(segs, sloopOnlySegments(marker)...)

	// hasOwnFooter: a wired statusline feed means the tool's own footer
	// already reports model/context/branch (its own script, or sloop's
	// freebie line if it had none — see statuslineFeedCmd). Two clean
	// directions from here: a footer already covers it → don't repeat it;
	// no footer (codex/cursor, driven by the heuristics.model pane-scan
	// instead, never have one) → the tmux bar is the only place to see it,
	// so show the full picture.
	if !statuslineInstalledFor(dir, tool, manifests[tool]) {
		segs = append(segs, fullAmbientSegments(dir, model, ctxPct)...)
	}

	return " ⚓ " + joinWith(statusSep, segs...) + " "
}

// sloopOnlySegments is ambient info nothing else on screen ever shows —
// currently just rate-limit usage — so it renders regardless of whether the
// tool has its own footer.
func sloopOnlySegments(marker fleetstate.State) []string {
	if rlPct, rlReset := marker.DisplayRateLimit(); rlPct > 0 {
		return []string{rateLimitSegment(rlPct, rlReset)}
	}
	return nil
}

// fullAmbientSegments is model/context/branch — shown only when the tool has
// no footer of its own to cover them (see renderStatuslineLeft).
func fullAmbientSegments(dir, model string, ctxPct int) []string {
	var segs []string
	if model != "" {
		segs = append(segs, "#[fg=colour140]"+activeIcons().Model+" "+model+"#[default]")
	}
	if ctxPct > 0 {
		segs = append(segs, contextSegment(ctxPct))
	}
	if branch := gitBranch(dir); branch != "" {
		segs = append(segs, "#[fg=colour110]"+activeIcons().Branch+" "+branch+"#[default]")
	}
	return segs
}

// renderStatuslineRight is the least-important side: a single rotating hint,
// dropped entirely below minWidthHint. Everything else — identity, status,
// model, context, branch — lives on the left, per renderStatuslineLeft.
func renderStatuslineRight(session string) string {
	if session == "" {
		return ""
	}
	if _, width := paneInfo(session); width < minWidthHint {
		return ""
	}
	h := rotatingHint(statusHints(), time.Now())
	if h == "" {
		return ""
	}
	return "#[fg=colour242]💡 " + h + "#[default] "
}

// ctxLevel is a context-usage urgency tier. It's the single place the warn/
// critical thresholds live, so every renderer that colors context% — this
// file's tmux segment and statuslinefeed.go's ANSI one — agrees on when the
// bar should worry the user.
type ctxLevel int

const (
	ctxNormal ctxLevel = iota
	ctxWarn            // filling up: compaction is getting closer
	ctxCrit            // compaction imminent
)

const (
	ctxWarnPct = 60
	ctxCritPct = 90
)

func classifyCtxPct(pct int) ctxLevel {
	switch {
	case pct >= ctxCritPct:
		return ctxCrit
	case pct >= ctxWarnPct:
		return ctxWarn
	default:
		return ctxNormal
	}
}

// ctxBarWidth is how many block characters wide the context-usage bar is —
// enough to show gradations without eating too much of the bar's real estate.
const ctxBarWidth = 8

// contextBar renders a block-character progress bar for context usage (e.g.
// "███░░░░░"), filled length proportional to pct. Shared by this file's tmux
// segment and statuslinefeed.go's ANSI one, so the two always agree on shape.
func contextBar(pct int) string {
	filled := min(pct*ctxBarWidth/100, ctxBarWidth)
	return strings.Repeat("█", filled) + strings.Repeat("░", ctxBarWidth-filled)
}

// contextSegment colors context usage by urgency: dim while comfortable,
// yellow past ctxWarnPct, red past ctxCritPct (compaction imminent).
func contextSegment(pct int) string {
	color := "colour245"
	switch classifyCtxPct(pct) {
	case ctxCrit:
		color = "red"
	case ctxWarn:
		color = "yellow"
	}
	return fmt.Sprintf("#[fg=%s]%s %s %d%%#[default]", color, activeIcons().Ctx, contextBar(pct), pct)
}

// rateLimitSegment renders "⏳ 24% (45m)" for the tmux bar, colored by
// urgency like context% (same thresholds — both are "usage%, higher is more
// urgent"). "" when unknown. This is new information no custom statusline
// script commonly surfaces, so it shows regardless of whether a feed is
// installed — unlike model/context/branch, it's never a duplicate.
func rateLimitSegment(pct int, resetIn string) string {
	if pct <= 0 {
		return ""
	}
	color := "colour245"
	switch classifyCtxPct(pct) {
	case ctxCrit:
		color = "red"
	case ctxWarn:
		color = "yellow"
	}
	label := fmt.Sprintf("%s %d%%", activeIcons().RateLimit, pct)
	if resetIn != "" {
		label += " (" + resetIn + ")"
	}
	return fmt.Sprintf("#[fg=%s]%s#[default]", color, label)
}

// statusHints is the rotating hint list, built from this server's live
// bindings so every hint names a key that actually works here.
func statusHints() []string {
	prefix := tmux.Prefix()
	hints := []string{"detach: " + prefix + " d"}
	if k := tmux.PeekKey(); k != "" {
		hints = append(hints, "peek: "+prefix+" "+k)
	}
	hints = append(hints, "fleet: sloop ps")
	return hints
}

// rotatingHint picks the hint for the current time slot; each hint holds for
// hintSlotSeconds so the bar changes occasionally without flickering.
func rotatingHint(hints []string, now time.Time) string {
	if len(hints) == 0 {
		return ""
	}
	return hints[int(now.Unix()/hintSlotSeconds)%len(hints)]
}

// extractModel extracts the current model from pane text using a manifest's
// heuristics.model pattern: last capture-group-1 match wins (a TUI's live
// footer sits at the bottom, below any stale intro header that may also name
// a model); "" on no match or a bad pattern.
func extractModel(text, pattern string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	ms := re.FindAllStringSubmatch(text, -1)
	if len(ms) == 0 || len(ms[len(ms)-1]) < 2 {
		return ""
	}
	return strings.TrimSpace(ms[len(ms)-1][1])
}

// paneInfo returns a session's active pane directory and its window width in
// one tmux round-trip. window_width tracks the attached client's size (and is
// the bar's own span), unlike client_width, which reports whichever client
// happens to be calling. Width 0 (unknown) renders as wide.
func paneInfo(session string) (dir string, width int) {
	out, err := tmux.Output("display-message", "-p", "-t", tmux.Exact(session), "#{pane_current_path}\t#{window_width}")
	if err != nil {
		return "", 0
	}
	path, w, _ := strings.Cut(strings.TrimSpace(string(out)), "\t")
	width, _ = strconv.Atoi(w)
	if width == 0 {
		width = 1 << 16 // unknown → don't drop segments
	}
	return path, width
}

// gitBranch reads the checked-out branch by walking to .git/HEAD directly —
// no subprocess, so it's cheap enough for a 2s status-interval. Handles
// worktrees/submodules (.git as a `gitdir:` file) and returns "" for detached
// HEAD or non-repos.
func gitBranch(dir string) string {
	if dir == "" {
		return ""
	}
	for d := dir; ; d = filepath.Dir(d) {
		if head := readHead(filepath.Join(d, ".git")); head != "" {
			return head
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

// readHead resolves a .git path (dir or worktree pointer file) to the branch
// named in HEAD, "" when it isn't a branch checkout.
func readHead(gitPath string) string {
	fi, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	gitDir := gitPath
	if !fi.IsDir() {
		b, err := os.ReadFile(gitPath)
		if err != nil {
			return ""
		}
		line := strings.TrimSpace(string(b))
		if !strings.HasPrefix(line, "gitdir:") {
			return ""
		}
		gitDir = strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(filepath.Dir(gitPath), gitDir)
		}
	}
	b, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(b))
	if ref, ok := strings.CutPrefix(head, "ref: refs/heads/"); ok {
		return ref
	}
	return "" // detached HEAD: no branch to show
}

// waitingBadge formats the fleet-wide waiting count for the status bar, empty
// when nothing is waiting so the bar stays clean. hint is the actionable suffix
// (e.g. " → Ctrl+b j") appended inside the badge, or "" when no peek key is bound.
func waitingBadge(n int, hint string) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf(" #[fg=yellow]⏳ %d waiting%s#[default]", n, hint)
}

// peekHint is the status-bar suffix that tells you which keystroke reaches the
// waiting agent, e.g. " → Ctrl+b j"; empty when no peek key is bound on this
// server. prefix is the human form ("Ctrl+b"); key is the bound peek key.
func peekHint(prefix, key string) string {
	if key == "" {
		return ""
	}
	return " → " + prefix + " " + key
}

// renderFleetBadge counts sloop sessions whose fresh marker is `waiting`,
// excluding the current session (so it reads "others need you"). Markers only,
// no capture-pane, because this re-renders every status-interval.
func renderFleetBadge(exclude string) string {
	n := 0
	for _, s := range tmux.ParseSessions(tmuxList()) {
		if s.Name == exclude {
			continue
		}
		if strings.LastIndex(s.Name, "__") < 0 {
			continue
		}
		if m, ok := fleetstate.Read(s.Name); ok && m.Status == "waiting" {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	return waitingBadge(n, peekHint(tmux.Prefix(), tmux.PeekKey()))
}

// statuslineSideRunE renders one side of the status bar to stdout. Must go to
// stdout: tmux's #() in the status bar captures stdout only (cobra's cmd.Print
// writes to stderr, which the status bar never sees).
func statuslineSideRunE(render func(string) string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		session := currentSession()
		if len(args) == 1 {
			session = args[0]
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), render(session))
		return nil
	}
}

var statuslineCmd = &cobra.Command{
	Use:   "statusline [session]",
	Short: "Manage the sloop status bar (install provider feeds, re-apply per session)",
	Long: `Manage the per-session status bar sloop puts on tmux.

The bar's left side shows this agent (status, workspace·tool, model, context
%, git branch) plus a badge when other agents wait; the right side is just a
rotating hint. Model and context come from the tool itself once
` + "`sloop statusline install <tool>`" + ` registers a feed there (SLOOP_NERD_FONTS=1
switches the icons to Nerd Font glyphs).

The bare command renders one side and is invoked by tmux, not by users.`,
	Args: cobra.MaximumNArgs(1),
	RunE: statuslineSideRunE(renderStatusline),
}

var statuslineLeftCmd = &cobra.Command{
	Use:    "left [session]",
	Short:  "Internal: render the status bar's left (attention) side",
	Hidden: true,
	Args:   cobra.MaximumNArgs(1),
	RunE:   statuslineSideRunE(renderStatuslineLeft),
}

var statuslineRightCmd = &cobra.Command{
	Use:    "right [session]",
	Short:  "Internal: render the status bar's right (ambient info) side",
	Hidden: true,
	Args:   cobra.MaximumNArgs(1),
	RunE:   statuslineSideRunE(renderStatuslineRight),
}

var statuslineSetupCmd = &cobra.Command{
	Use:   "setup [session]",
	Short: "Show sloop's status bar in a session (default: the current one)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tmux.Available() {
			return fmt.Errorf("`sloop statusline setup` needs tmux")
		}
		session := currentSession()
		if len(args) == 1 {
			session = args[0]
		}
		if session == "" {
			return fmt.Errorf("run inside a tmux session, or pass a session name")
		}
		tmux.SetStatusLine(session)
		cmd.Printf("status bar enabled for %s\n", session)
		return nil
	},
}

func RegisterStatusline(cmd *cobra.Command) {
	statuslineFeedCmd.Flags().StringVar(&feedChain, "chain", "", "statusline command to pass the payload through to (its output is shown)")
	statuslineInstallCmd.ValidArgs = hookTools()
	statuslineCmd.AddCommand(statuslineLeftCmd)
	statuslineCmd.AddCommand(statuslineRightCmd)
	statuslineCmd.AddCommand(statuslineSetupCmd)
	statuslineCmd.AddCommand(statuslineFeedCmd)
	statuslineCmd.AddCommand(statuslineInstallCmd)
	statuslineCmd.ValidArgsFunction = completeSessionNames
	cmd.AddCommand(statuslineCmd)
}
