// Package update detects when a newer sloop release is available and reports it
// without ever slowing a command down. The model mirrors how a good shell tool
// does it: a command reads a small cached result (instant) and, only if that
// cache is stale, spawns a detached background process to refresh it for next
// time. The network is never on the critical path of an interactive command.
package update

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/tui"
)

const (
	// repo is the GitHub repository sloop releases ship from.
	repo = "stroops/sloop"
	// cacheFileName is the per-user cache of the last update check, under ~/.sloop.
	cacheFileName = "update_check.json"
	// staleAfter is how long a cached check is trusted before a background
	// refresh is triggered. Kept generous so we hit GitHub at most ~once a day.
	staleAfter = 24 * time.Hour
	// fetchTimeout bounds the background HTTP call so a hung network can never
	// leave a lingering process.
	fetchTimeout = 5 * time.Second
)

// state is the cached result of the most recent successful (or attempted) check.
type state struct {
	Latest    string    `json:"latest"`
	CheckedAt time.Time `json:"checked_at"`
}

// IsRelease reports whether v looks like a real release version (as injected by
// goreleaser) rather than a local/dev build. Update checks are pointless — and
// noisy — for `dev`, `make build`, or `go install`-from-source binaries.
func IsRelease(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || v == "dev" {
		return false
	}
	// A release version starts with a digit (e.g. "0.1.2"). Snapshot builds look
	// like "0.1.3-next"; treat those as non-releases too.
	return len(v) > 0 && v[0] >= '0' && v[0] <= '9' && !strings.Contains(v, "next")
}

func cachePath() (string, error) {
	dir, err := config.GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cacheFileName), nil
}

func readState() (state, bool) {
	p, err := cachePath()
	if err != nil {
		return state{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return state{}, false
	}
	var s state
	if err := json.Unmarshal(b, &s); err != nil {
		return state{}, false
	}
	return s, true
}

func writeState(s state) error {
	dir, err := config.GlobalDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	p := filepath.Join(dir, cacheFileName)
	return os.WriteFile(p, b, 0o600)
}

// Status reports the latest known version and whether it is newer than current,
// reading only the local cache (no network). available is false for dev builds
// or when no fresh-enough newer version is cached.
func Status(current string) (latest string, available bool) {
	if !IsRelease(current) {
		return "", false
	}
	s, ok := readState()
	if !ok || s.Latest == "" {
		return "", false
	}
	return s.Latest, less(normalize(current), normalize(s.Latest))
}

// Banner returns a one-line, colored update notice for the home menu and `ps`
// header, or "" when no update is available.
func Banner(current string) string {
	latest, available := Status(current)
	if !available {
		return ""
	}
	return tui.Yellow("⬆ Update "+latest+" available") +
		tui.Grey(" — run ") + tui.Bold("sloop update")
}

// MaybeTriggerBackground refreshes the cache out of band when it is missing or
// stale. It returns immediately: the actual network call runs in a detached
// child process (`sloop internal-update-check`) that outlives this command, so
// the result is ready for the next invocation. No-op for dev builds.
func MaybeTriggerBackground(current string) {
	if !IsRelease(current) {
		return
	}
	if s, ok := readState(); ok && time.Since(s.CheckedAt) < staleAfter {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "internal-update-check")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.Env = append(os.Environ(), "SLOOP_INTERNAL_UPDATE_CHECK=1")
	detach(cmd) // platform-specific: start in its own session so it isn't killed
	if err := cmd.Start(); err != nil {
		return
	}
	// Release the process so we don't accumulate a zombie; we never Wait on it.
	_ = cmd.Process.Release()
}

// RunCheck performs the network fetch and writes the cache. It is invoked by the
// hidden `internal-update-check` command in the detached child. It always
// records the attempt time so a failed fetch still throttles future checks.
func RunCheck(current string) {
	s, _ := readState()
	s.CheckedAt = time.Now()
	if latest, err := fetchLatest(); err == nil && latest != "" {
		s.Latest = latest
	}
	_ = writeState(s)
}

// fetchLatest asks GitHub for the most recent release tag (e.g. "v0.1.3").
func fetchLatest() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sloop-update-check")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", &httpError{resp.StatusCode}
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return strings.TrimPrefix(body.TagName, "v"), nil
}

type httpError struct{ code int }

func (e *httpError) Error() string { return "github: status " + strconv.Itoa(e.code) }

// normalize strips a leading "v" and any pre-release/build suffix, leaving the
// dotted numeric core (e.g. "v0.1.3-next" -> "0.1.3").
func normalize(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return v
}

// less reports whether normalized version a is strictly older than b, comparing
// dotted numeric components. Non-numeric or missing components compare as 0, so
// it degrades gracefully on unexpected input rather than panicking.
func less(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	n := max(len(pa), len(pb))
	for i := range n {
		na, nb := part(pa, i), part(pb, i)
		if na != nb {
			return na < nb
		}
	}
	return false
}

func part(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, _ := strconv.Atoi(parts[i])
	return n
}
