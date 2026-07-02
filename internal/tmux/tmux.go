package tmux

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/stroops/sloop/internal/runner"
)

// Run / Output / OutputContext execute a multiplexer subcommand, logging the
// call (and any error) at debug level so `--debug` shows exactly what sloop
// asked tmux/psmux to do, the first thing you want when a fleet command
// misbehaves. All sloop multiplexer access goes through these.
func Run(args ...string) error {
	slog.Debug("mux", "bin", Bin(), "args", args)
	if err := exec.Command(Bin(), args...).Run(); err != nil {
		slog.Debug("mux failed", "args", args, "err", err)
		return err
	}
	return nil
}

func Output(args ...string) ([]byte, error) {
	return OutputContext(context.Background(), args...)
}

func OutputContext(ctx context.Context, args ...string) ([]byte, error) {
	slog.Debug("mux", "bin", Bin(), "args", args)
	out, err := exec.CommandContext(ctx, Bin(), args...).Output()
	if err != nil {
		slog.Debug("mux failed", "args", args, "err", err)
	}
	return out, err
}

// muxCandidates are the tmux-compatible multiplexers sloop drives, in
// preference order. psmux (native Windows, Rust) speaks the same CLI as tmux,
// so the whole backend works on Windows by just picking a different binary.
var muxCandidates = []string{"tmux", "psmux"}

var (
	binOnce sync.Once
	binName string
)

// Bin returns the multiplexer binary to invoke: $SLOOP_MUX if set, else the
// first of tmux/psmux found on PATH, else "tmux" (so callers still get a
// sensible command; Available() reports whether it actually exists).
func Bin() string {
	binOnce.Do(func() {
		binName = resolveBin(os.Getenv("SLOOP_MUX"), func(c string) bool {
			_, err := exec.LookPath(c)
			return err == nil
		})
	})
	return binName
}

// resolveBin is the pure selection logic behind Bin (testable without PATH).
func resolveBin(env string, onPath func(string) bool) string {
	if env != "" {
		return env
	}
	for _, c := range muxCandidates {
		if onPath(c) {
			return c
		}
	}
	return "tmux"
}

// Available reports whether a usable multiplexer (tmux or psmux) is installed.
func Available() bool {
	_, err := exec.LookPath(Bin())
	return err == nil
}

func SessionName(workspace, tool string) string {
	return sanitize(workspace) + "__" + sanitize(tool)
}

// Exact turns a session name into an exact-match tmux target ("=name"). A bare
// `-t name` falls back to prefix matching when no session has that exact name,
// so with only ws__claude__sec running, `-t ws__claude` silently resolves to it
// (wrong attach/send/kill). Every `-t` that means one whole session by name
// must go through this. psmux strips a leading "=" and always matches session
// names exactly (parse_target in its cli.rs), so the prefix is safe there too.
func Exact(session string) string { return "=" + session }

// InstanceName is SessionName plus an optional instance suffix; instance=="" is
// the default session (byte-identical to SessionName), so a second agent of the
// same provider in one workspace gets a distinct `ws__tool__instance` name.
func InstanceName(workspace, tool, instance string) string {
	base := SessionName(workspace, tool)
	if instance == "" {
		return base
	}
	return base + "__" + sanitize(instance)
}

// envPrefix returns ["env","K1=V1",…] in sorted key order, or nil when empty, so
// a launched command can carry extra env (e.g. CLAUDE_CONFIG_DIR for a second
// account) on any tmux version, with no dependency on `new-session -e`.
func envPrefix(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := []string{"env"}
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

// buildNewDetachedArgs builds `new-session -d -s <s> -c <dir> [env K=V…] cmd args…`.
func buildNewDetachedArgs(session, dir string, env map[string]string, command string, cmdArgs []string) []string {
	args := []string{"new-session", "-d", "-s", session, "-c", dir}
	args = append(args, envPrefix(env)...)
	args = append(args, command)
	return append(args, cmdArgs...)
}

// DetachHint is the one place that explains how to detach (hide the agent and
// return to the terminal), using the user's actual tmux prefix. Shared by run,
// attach, and ps so the wording stays consistent.
func DetachHint() string {
	return fmt.Sprintf("\033[36m💡 detach (hide this agent, keep it running): press \033[1m%s\033[0m\033[36m then \033[1md\033[0m", Prefix())
}

// DetachLine is a compact one-liner for menu/list footers.
func DetachLine() string {
	return fmt.Sprintf("detach: %s then d", Prefix())
}

func Prefix() string {
	out, err := Output("show-options", "-g", "prefix")
	if err != nil {
		return "Ctrl+b"
	}
	s := strings.TrimSpace(string(out))
	parts := strings.Split(s, " ")
	if len(parts) == 2 {
		p := parts[1]
		if strings.HasPrefix(p, "C-") {
			return "Ctrl+" + p[2:]
		}
		return p
	}
	return "Ctrl+b"
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func BuildAttachArgs(session string) []string {
	return []string{"attach", "-t", Exact(session)}
}

func BuildKillArgs(session string) []string {
	return []string{"kill-session", "-t", Exact(session)}
}

// Kill ends a session (the agent stops with it).
func Kill(session string) error {
	return Run(BuildKillArgs(session)...)
}

func BuildRenameArgs(old, name string) []string {
	return []string{"rename-session", "-t", Exact(old), name}
}

// Rename renames a running session (used to adopt an external session into the
// sloop `<workspace>__<tool>` convention).
func Rename(old, name string) error {
	return Run(BuildRenameArgs(old, name)...)
}

// SessionPath reports a session's active pane working directory (best-effort),
// so an adopted session can be registered against its repo path.
func SessionPath(session string) string {
	out, err := Output("display-message", "-t", Exact(session), "-p", "#{pane_current_path}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// LaunchDetached creates a detached session running command+args in dir (with
// extra env for the launched process) and gives it sloop's per-session status
// bar, without attaching. It reports whether it created the session (false: it
// already existed and nothing was done). The single create-detached path,
// shared by DetachedRunner (`sloop new`) and `sloop restore`.
func LaunchDetached(session, dir string, env map[string]string, command string, args []string) (bool, error) {
	if HasSession(session) {
		return false, nil
	}
	create := buildNewDetachedArgs(session, dir, env, command, args)
	if err := Run(create...); err != nil {
		return false, err
	}
	SetStatusLine(session)
	EnsureFleetKeys()
	return true, nil
}

// AlreadyRunningMsg / CreatedDetachedMsg are the single source of the two
// `sloop new` outcome lines, so the command layer and DetachedRunner can never
// drift apart in wording.
func AlreadyRunningMsg(session string) string {
	return fmt.Sprintf("session %s is already running — attach: sloop attach %s", session, session)
}

func CreatedDetachedMsg(session string) string {
	return fmt.Sprintf("created %s (detached) — attach: sloop attach %s", session, session)
}

// DetachedRunner creates the session without attaching (`sloop new`): the
// agent starts in the background and the terminal stays free.
type DetachedRunner struct {
	Session string
	Out     io.Writer // destination of the outcome line (nil = stdout)
	created bool
}

// Detached reports whether Launch left a session running in the background:
// true after it creates one (the session history row must stay open), false
// when the session already existed and nothing was launched.
func (r *DetachedRunner) Detached() bool { return r.created }

func (r *DetachedRunner) Launch(s runner.Spec) error {
	created, err := LaunchDetached(r.Session, s.Dir, s.Env, s.Command, s.Args)
	if err != nil {
		return err
	}
	r.created = created
	out := r.Out
	if out == nil {
		out = os.Stdout
	}
	if created {
		_, _ = fmt.Fprintln(out, CreatedDetachedMsg(r.Session))
	} else {
		_, _ = fmt.Fprintln(out, AlreadyRunningMsg(r.Session))
	}
	return nil
}

type Runner struct {
	Session string
}

func (r Runner) Launch(s runner.Spec) error {
	fmt.Printf("\n%s\n\n", DetachHint())

	// Create the session detached so we can give it sloop's own status bar
	// before attaching; then attach (or switch if already inside tmux).
	if _, err := LaunchDetached(r.Session, s.Dir, s.Env, s.Command, s.Args); err != nil {
		return err
	}
	args := BuildAttachArgs(r.Session)
	if os.Getenv("TMUX") != "" {
		args = BuildSwitchArgs(r.Session)
	}
	cmd := exec.Command(Bin(), args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
