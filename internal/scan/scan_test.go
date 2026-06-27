package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasCmd(r Report, cmd string) bool {
	for _, c := range r.Commands {
		if c.Cmd == cmd {
			return true
		}
	}
	return false
}

func hasLang(r Report, name string) bool {
	for _, l := range r.Languages {
		if l.Name == name {
			return true
		}
	}
	return false
}

func TestScanGoRepo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module github.com/acme/widget\n\ngo 1.26\n")
	if err := os.MkdirAll(filepath.Join(dir, "cmd"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "internal"), 0o700); err != nil {
		t.Fatal(err)
	}
	r := Scan(dir)
	if r.Name != "widget" {
		t.Fatalf("name = %q, want widget", r.Name)
	}
	if !hasLang(r, "Go") {
		t.Fatalf("languages = %+v, want Go", r.Languages)
	}
	if !hasCmd(r, "go test ./...") {
		t.Fatalf("commands = %+v, want go test ./...", r.Commands)
	}
	wantDir := map[string]bool{"cmd": false, "internal": false}
	for _, d := range r.Layout {
		if _, ok := wantDir[d]; ok {
			wantDir[d] = true
		}
	}
	for d, found := range wantDir {
		if !found {
			t.Fatalf("layout missing %q: %+v", d, r.Layout)
		}
	}
}

func TestScanNodeTSRepo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"name":"webapp","scripts":{"build":"tsc","test":"vitest"}}`)
	write(t, dir, "tsconfig.json", "{}")
	r := Scan(dir)
	if r.Name != "webapp" {
		t.Fatalf("name = %q", r.Name)
	}
	if !hasLang(r, "TypeScript") {
		t.Fatalf("languages = %+v, want TypeScript", r.Languages)
	}
	if !hasCmd(r, "npm run build") || !hasCmd(r, "npm test") {
		t.Fatalf("commands = %+v", r.Commands)
	}
}

func TestScanMakefileWins(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module x\n\ngo 1.26\n")
	write(t, dir, "Makefile", "test:\n\tgo test ./...\nbuild:\n\tgo build ./...\n")
	r := Scan(dir)
	if !hasCmd(r, "make test") {
		t.Fatalf("Makefile target should win: %+v", r.Commands)
	}
}

func TestScanUnknownRepo(t *testing.T) {
	dir := t.TempDir()
	r := Scan(dir)
	if r.Name != filepath.Base(dir) {
		t.Fatalf("name = %q, want basename", r.Name)
	}
	if len(r.Languages) != 0 {
		t.Fatalf("want no languages, got %+v", r.Languages)
	}
}

func TestScanReadmeSeed(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "README.md", "# Widget\n\nWidget is a fast thing.\n")
	if got := Scan(dir).Summary; got != "Widget is a fast thing." {
		t.Fatalf("summary = %q", got)
	}
}
