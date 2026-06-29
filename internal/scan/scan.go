// Package scan reads an existing repository (marker files + a shallow directory
// listing) and produces a Report used to pre-fill AGENTS.md. It is deterministic,
// best-effort, and never shells out or walks the tree.
package scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Report struct {
	Name      string
	Languages []Lang
	Commands  []Command
	Layout    []string
	Summary   string
}

type Lang struct{ Name, Version string }

type Command struct{ Label, Cmd string } // build | test | lint | run

func Scan(root string) Report {
	langs := languages(root)
	return Report{
		Name:      projectName(root),
		Languages: langs,
		Commands:  commands(root, langs),
		Layout:    layout(root),
		Summary:   readmeSeed(root),
	}
}

func projectName(root string) string {
	if mod := goModModule(root); mod != "" {
		return lastPathElem(mod)
	}
	if n := jsonString(parseJSON(filepath.Join(root, "package.json")), "name"); n != "" {
		return n
	}
	if n := tomlName(filepath.Join(root, "Cargo.toml")); n != "" {
		return n
	}
	if n := tomlName(filepath.Join(root, "pyproject.toml")); n != "" {
		return n
	}
	return filepath.Base(root)
}

func languages(root string) []Lang {
	var out []Lang
	if exists(root, "go.mod") {
		out = append(out, Lang{"Go", goVersion(root)})
	}
	if exists(root, "package.json") {
		name := "JavaScript"
		if exists(root, "tsconfig.json") {
			name = "TypeScript"
		}
		out = append(out, Lang{name, nodeVersion(root)})
	}
	if exists(root, "Cargo.toml") {
		out = append(out, Lang{"Rust", ""})
	}
	if exists(root, "pyproject.toml") || exists(root, "setup.py") || exists(root, "requirements.txt") {
		out = append(out, Lang{"Python", pythonVersion(root)})
	}
	if exists(root, "pom.xml") || exists(root, "build.gradle") || exists(root, "build.gradle.kts") {
		out = append(out, Lang{"Java/Kotlin", ""})
	}
	if exists(root, "Gemfile") {
		out = append(out, Lang{"Ruby", ""})
	}
	if exists(root, "composer.json") {
		out = append(out, Lang{"PHP", ""})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func commands(root string, langs []Lang) []Command {
	m := map[string]string{}
	for _, l := range langs {
		switch l.Name {
		case "Go":
			set(m, "build", "go build ./...")
			set(m, "test", "go test ./...")
			lint := "go vet ./..."
			if exists(root, ".golangci.yml") || exists(root, ".golangci.yaml") {
				lint = "golangci-lint run"
			}
			set(m, "lint", lint)
		case "Rust":
			set(m, "build", "cargo build")
			set(m, "test", "cargo test")
			set(m, "lint", "cargo clippy")
		case "Python":
			set(m, "test", "python -m pytest")
		case "JavaScript", "TypeScript":
			pm := nodePM(root)
			if hasScript(root, "build") {
				set(m, "build", pm+" run build")
			}
			if hasScript(root, "test") {
				set(m, "test", pm+" test")
			}
			if hasScript(root, "lint") {
				set(m, "lint", pm+" run lint")
			}
		}
	}
	// A Makefile declares the project's own canonical commands, so it wins.
	for _, label := range []string{"build", "test", "lint", "run"} {
		if makefileHasTarget(root, label) {
			m[label] = "make " + label
		}
	}
	var out []Command
	for _, label := range []string{"build", "test", "lint", "run"} {
		if c, ok := m[label]; ok {
			out = append(out, Command{label, c})
		}
	}
	return out
}

var meaningfulDirs = map[string]bool{
	"cmd": true, "internal": true, "pkg": true, "src": true, "app": true,
	"lib": true, "test": true, "tests": true, "docs": true, "api": true,
	"web": true, "server": true, "client": true, "services": true, "packages": true,
}

func layout(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && meaningfulDirs[e.Name()] {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// readmeSeed returns the first non-empty, non-heading, non-badge line of README.md.
func readmeSeed(root string) string {
	for _, ln := range strings.Split(readFile(filepath.Join(root, "README.md")), "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "![") || strings.HasPrefix(t, "[!") {
			continue
		}
		t = strings.TrimLeft(t, "> ") // unwrap a blockquote tagline
		if t != "" {
			return t
		}
	}
	return ""
}

// --- best-effort file helpers (never error) ---

func exists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func set(m map[string]string, k, v string) {
	if _, ok := m[k]; !ok {
		m[k] = v
	}
}

func lastPathElem(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

func goModModule(root string) string {
	for _, ln := range strings.Split(readFile(filepath.Join(root, "go.mod")), "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(ln, "module"))
		}
	}
	return ""
}

func goVersion(root string) string {
	for _, ln := range strings.Split(readFile(filepath.Join(root, "go.mod")), "\n") {
		f := strings.Fields(ln)
		if len(f) == 2 && f[0] == "go" {
			return f[1]
		}
	}
	return ""
}

func pythonVersion(root string) string {
	return strings.TrimSpace(strings.SplitN(readFile(filepath.Join(root, ".python-version")), "\n", 2)[0])
}

func parseJSON(path string) map[string]any {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

func jsonString(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func nodeVersion(root string) string {
	m := parseJSON(filepath.Join(root, "package.json"))
	if eng, ok := m["engines"].(map[string]any); ok {
		if v, ok := eng["node"].(string); ok {
			return v
		}
	}
	return ""
}

func hasScript(root, name string) bool {
	m := parseJSON(filepath.Join(root, "package.json"))
	scripts, ok := m["scripts"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = scripts[name]
	return ok
}

func nodePM(root string) string {
	switch {
	case exists(root, "pnpm-lock.yaml"):
		return "pnpm"
	case exists(root, "yarn.lock"):
		return "yarn"
	default:
		return "npm"
	}
}

func tomlName(path string) string {
	for _, ln := range strings.Split(readFile(path), "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "name") {
			if i := strings.Index(ln, "="); i >= 0 {
				if v := strings.Trim(strings.TrimSpace(ln[i+1:]), "\"'"); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

func makefileHasTarget(root, target string) bool {
	for _, ln := range strings.Split(readFile(filepath.Join(root, "Makefile")), "\n") {
		if strings.HasPrefix(ln, target+":") {
			return true
		}
	}
	return false
}
