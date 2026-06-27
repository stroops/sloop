package cli

import (
	"log/slog"
	"os"
)

// setupLogging configures the global slog logger. By default sloop is quiet
// (warnings+ only) — user-facing output goes through cmd.Print, not the log.
// `--debug` or `SLOOP_DEBUG` turns on debug-level diagnostics to stderr, which
// is what you reach for when something best-effort (a tmux call, a DB write)
// fails silently in a release build.
func setupLogging(debug bool) {
	level := slog.LevelWarn
	if debug || os.Getenv("SLOOP_DEBUG") != "" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}
