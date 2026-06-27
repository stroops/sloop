package hints

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestNormalizeLang(t *testing.T) {
	for in, want := range map[string]string{
		"vi_VN.UTF-8": "vi",
		"en-US":       "en",
		"EN":          "en",
		"vi":          "vi",
	} {
		if got := normalizeLang(in); got != want {
			t.Fatalf("normalizeLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocalizedFallback(t *testing.T) {
	h := Hint{Text: map[string]string{"en": "hello"}}
	if got := h.Localized("vi"); got != "hello" {
		t.Fatalf("fallback to en failed: %q", got)
	}
	h.Text["vi"] = "xin chào"
	if got := h.Localized("vi"); got != "xin chào" {
		t.Fatalf("vi = %q", got)
	}
}

func TestPickPrefersContextThenLeastRecent(t *testing.T) {
	hs := []Hint{
		{ID: "a", Context: "ps"},
		{ID: "b", Context: "ps"},
		{ID: "g", Context: "general"},
	}
	now := time.Unix(1_000_000_000, 0)
	// a shown recently, b never → pick b.
	st := state{Shown: map[string]int64{"a": now.Unix() - 60}}
	got, ok := pick(hs, "ps", st, now)
	if !ok || got.ID != "b" {
		t.Fatalf("want b, got %q ok=%v", got.ID, ok)
	}
	// No context match → fall back to general.
	got, ok = pick(hs, "sync", st, now)
	if !ok || got.ID != "g" {
		t.Fatalf("want general g, got %q ok=%v", got.ID, ok)
	}
	// All ps hints on cooldown → none.
	st.Shown["a"] = now.Unix()
	st.Shown["b"] = now.Unix()
	if _, ok := pick(hs, "ps", st, now); ok {
		t.Fatal("expected no eligible ps hint")
	}
}

func TestShowThrottleAndDisable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SLOOP_LANG", "en")
	fixed := time.Unix(2_000_000_000, 0)
	Now = func() time.Time { return fixed }
	defer func() { Now = time.Now }()

	var b bytes.Buffer
	Show(&b, "init")
	if !strings.Contains(b.String(), "💡") {
		t.Fatalf("expected a hint, got %q", b.String())
	}
	// Immediate second call → global cooldown → nothing.
	b.Reset()
	Show(&b, "tools")
	if b.Len() != 0 {
		t.Fatalf("expected cooldown silence, got %q", b.String())
	}
	// Disabled via env.
	t.Setenv("SLOOP_NO_HINTS", "1")
	b.Reset()
	Now = func() time.Time { return fixed.Add(time.Hour) }
	Show(&b, "init")
	if b.Len() != 0 {
		t.Fatalf("expected silence when disabled, got %q", b.String())
	}
}
