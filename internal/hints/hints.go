// Package hints shows occasional, contextual education tips after sloop
// commands, demystifying tmux/CLI for newcomers. Hints are embedded (offline,
// ship with each release), localized (en/vi today), throttled so they never
// spam, and easy to turn off. A future registry/global-DB overlay can extend
// Load() without changing callers.
package hints

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/stroops/sloop/internal/config"
	"github.com/stroops/sloop/internal/tui"
)

//go:embed hints.yaml
var hintsYAML []byte

// Hint is one tip: a context (command name or "general") and per-language text.
type Hint struct {
	ID      string            `yaml:"id"`
	Context string            `yaml:"context"`
	Text    map[string]string `yaml:"text"`
}

type hintFile struct {
	Hints []Hint `yaml:"hints"`
}

var (
	loadOnce sync.Once
	all      []Hint
)

// Load returns the embedded hints (parsed once).
func Load() []Hint {
	loadOnce.Do(func() {
		var f hintFile
		_ = yaml.Unmarshal(hintsYAML, &f)
		all = f.Hints
	})
	return all
}

// Now is overridable in tests.
var Now = time.Now

const (
	globalCooldown  = 5 * time.Minute    // don't nag on every consecutive command
	perHintCooldown = 7 * 24 * time.Hour // don't repeat a given hint too soon
)

func enabled() bool {
	if os.Getenv("SLOOP_NO_HINTS") != "" {
		return false
	}
	if g, err := config.LoadGlobal(); err == nil && g.Hints != nil && !*g.Hints {
		return false
	}
	return true
}

// Lang resolves the display language: SLOOP_LANG → config lang → LANG → "en".
func Lang() string {
	if l := os.Getenv("SLOOP_LANG"); l != "" {
		return normalizeLang(l)
	}
	if g, err := config.LoadGlobal(); err == nil && g.Lang != "" {
		return normalizeLang(g.Lang)
	}
	if l := os.Getenv("LANG"); l != "" {
		return normalizeLang(l)
	}
	return "en"
}

// normalizeLang turns "vi_VN.UTF-8" / "en-US" into "vi" / "en".
func normalizeLang(l string) string {
	l = strings.ToLower(strings.TrimSpace(l))
	if i := strings.IndexAny(l, "_-."); i >= 0 {
		l = l[:i]
	}
	return l
}

// Localized returns the hint text for lang, falling back to English.
func (h Hint) Localized(lang string) string {
	if t, ok := h.Text[lang]; ok && t != "" {
		return t
	}
	return h.Text["en"]
}

type state struct {
	Last  int64            `json:"last"`
	Shown map[string]int64 `json:"shown"`
}

func statePath() (string, error) {
	d, err := config.GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "hints-state.json"), nil
}

func loadState() state {
	st := state{Shown: map[string]int64{}}
	p, err := statePath()
	if err != nil {
		return st
	}
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if st.Shown == nil {
		st.Shown = map[string]int64{}
	}
	return st
}

func saveState(st state) {
	p, err := statePath()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	if b, err := json.Marshal(st); err == nil {
		_ = os.WriteFile(p, b, 0o600)
	}
}

// pick chooses the least-recently-shown eligible hint for a context (falling
// back to "general"), skipping ones shown within perHintCooldown. Pure, so the
// selection logic is unit-tested without the filesystem.
func pick(hints []Hint, context string, st state, now time.Time) (Hint, bool) {
	cands := byContext(hints, context)
	if len(cands) == 0 {
		cands = byContext(hints, "general")
	}
	var best Hint
	var bestSeen int64 = 1 << 62
	found := false
	for _, h := range cands {
		seen := st.Shown[h.ID]
		if now.Unix()-seen < int64(perHintCooldown.Seconds()) {
			continue
		}
		if seen < bestSeen {
			bestSeen, best, found = seen, h, true
		}
	}
	return best, found
}

func byContext(hints []Hint, context string) []Hint {
	var out []Hint
	for _, h := range hints {
		if h.Context == context {
			out = append(out, h)
		}
	}
	return out
}

// Show prints one contextual hint to w, respecting opt-out and throttling. It is
// best-effort: any disabled/throttled/error condition simply prints nothing.
func Show(w io.Writer, context string) {
	if !enabled() {
		return
	}
	now := Now()
	st := loadState()
	if now.Unix()-st.Last < int64(globalCooldown.Seconds()) {
		return
	}
	h, ok := pick(Load(), context, st, now)
	if !ok {
		return
	}
	_, _ = fmt.Fprintln(w, tui.Grey("💡 "+h.Localized(Lang())))
	st.Last = now.Unix()
	st.Shown[h.ID] = now.Unix()
	saveState(st)
}
