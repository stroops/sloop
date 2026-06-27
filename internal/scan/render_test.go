package scan

import (
	"strings"
	"testing"
)

func TestAgentsMarkdownPopulated(t *testing.T) {
	r := Report{
		Name:      "widget",
		Languages: []Lang{{Name: "Go", Version: "1.26"}},
		Commands:  []Command{{Label: "test", Cmd: "go test ./..."}},
		Layout:    []string{"cmd", "internal"},
		Summary:   "A widget service.",
	}
	md := r.AgentsMarkdown()
	for _, want := range []string{"# AGENTS.md", "widget", "Go 1.26", "go test ./...", "internal", "## Conventions", "A widget service."} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestAgentsMarkdownEmptyStillValid(t *testing.T) {
	md := Report{Name: "x"}.AgentsMarkdown()
	if !strings.Contains(md, "# AGENTS.md") || !strings.Contains(md, "## Conventions") {
		t.Fatalf("empty report markdown invalid:\n%s", md)
	}
	// No detected sections when nothing was found.
	if strings.Contains(md, "## Tech stack") || strings.Contains(md, "## Build") {
		t.Fatalf("empty report should omit detected sections:\n%s", md)
	}
}
