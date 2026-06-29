package update

import (
	"path/filepath"
	"testing"
	"time"
)

func TestIsRelease(t *testing.T) {
	cases := map[string]bool{
		"":           false,
		"dev":        false,
		"0.1.3-next": false,
		"0.1.2":      true,
		"1.0.0":      true,
		"v0.1.2":     false, // normalize happens elsewhere; raw "v" isn't a release tag here
		"snapshot":   false,
	}
	for in, want := range cases {
		if got := IsRelease(in); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"v0.1.2":      "0.1.2",
		"0.1.2":       "0.1.2",
		"v0.1.3-next": "0.1.3",
		"1.2.3+build": "1.2.3",
		"  v2.0.0 ":   "2.0.0",
	}
	for in, want := range cases {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.1.2", "0.1.3", true},
		{"0.1.3", "0.1.2", false},
		{"0.1.2", "0.1.2", false},
		{"0.1.2", "0.2.0", true},
		{"0.9.0", "1.0.0", true},
		{"1.0", "1.0.1", true},
		{"1.0.1", "1.0", false},
		{"0.10.0", "0.9.0", false}, // numeric, not lexical: 10 > 9
		{"0.9.0", "0.10.0", true},
	}
	for _, c := range cases {
		if got := less(c.a, c.b); got != c.want {
			t.Errorf("less(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestStatusReadsCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// No cache yet: nothing available.
	if latest, avail := Status("0.1.2"); avail || latest != "" {
		t.Fatalf("with no cache: got (%q, %v), want (\"\", false)", latest, avail)
	}

	// Write a cache advertising a newer release.
	if err := writeState(state{Latest: "0.1.5", CheckedAt: time.Now()}); err != nil {
		t.Fatalf("writeState: %v", err)
	}
	latest, avail := Status("0.1.2")
	if !avail || latest != "0.1.5" {
		t.Errorf("newer cached: got (%q, %v), want (0.1.5, true)", latest, avail)
	}

	// Same version: not available.
	if _, avail := Status("0.1.5"); avail {
		t.Error("equal version should not report an update available")
	}

	// Dev build never reports an update even with a cache present.
	if _, avail := Status("dev"); avail {
		t.Error("dev build should never report an update available")
	}

	// Banner mirrors Status.
	if Banner("dev") != "" {
		t.Error("Banner(dev) should be empty")
	}
	if Banner("0.1.2") == "" {
		t.Error("Banner should be non-empty when an update is available")
	}
}

func TestCachePathUnderHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	p, err := cachePath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, ".sloop", cacheFileName); p != want {
		t.Errorf("cachePath = %q, want %q", p, want)
	}
}

func TestMaybeTriggerBackgroundFreshCacheNoop(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	// A fresh cache should suppress a new background spawn. We can't easily
	// observe the spawn, but we can assert the freshness gate by confirming the
	// cache is left untouched (CheckedAt unchanged) after the call.
	fresh := state{Latest: "0.1.5", CheckedAt: time.Now()}
	if err := writeState(fresh); err != nil {
		t.Fatal(err)
	}
	MaybeTriggerBackground("0.1.2") // should early-return on fresh cache
	got, ok := readState()
	if !ok || !got.CheckedAt.Equal(fresh.CheckedAt) {
		t.Error("fresh cache should not be rewritten by MaybeTriggerBackground")
	}
}
