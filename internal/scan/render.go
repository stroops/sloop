package scan

import (
	"fmt"
	"strings"
)

// AgentsMarkdown renders the scanned report as a populated AGENTS.md scaffold.
// Detected facts are tagged "_(detected)_"; sections with no content are dropped,
// except "## Conventions" which is always present as the key fill-in prompt.
func (r Report) AgentsMarkdown() string {
	var b strings.Builder
	b.WriteString("# AGENTS.md\n\n")
	b.WriteString("Project guidance for AI coding tools. This file is the canonical context;\n")
	b.WriteString("sloop points other tools (CLAUDE.md, GEMINI.md, …) at it.\n\n")

	name := r.Name
	if name == "" {
		name = "this project"
	}
	b.WriteString("## Project\n\n")
	fmt.Fprintf(&b, "**%s** — <!-- one-line description -->\n", name)
	if r.Summary != "" {
		fmt.Fprintf(&b, "\n> %s\n", r.Summary)
	}
	b.WriteString("\n")

	if len(r.Languages) > 0 {
		b.WriteString("## Tech stack\n\n")
		for _, l := range r.Languages {
			if l.Version != "" {
				fmt.Fprintf(&b, "- %s %s _(detected)_\n", l.Name, l.Version)
			} else {
				fmt.Fprintf(&b, "- %s _(detected)_\n", l.Name)
			}
		}
		b.WriteString("\n")
	}

	if len(r.Commands) > 0 {
		b.WriteString("## Build, test & lint\n\n```sh\n")
		for _, c := range r.Commands {
			fmt.Fprintf(&b, "%s\n", c.Cmd)
		}
		b.WriteString("```\n\n")
	}

	if len(r.Layout) > 0 {
		b.WriteString("## Project structure\n\n")
		for _, d := range r.Layout {
			fmt.Fprintf(&b, "- `%s/`\n", d)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Conventions\n\n")
	b.WriteString("<!-- Add coding standards, architectural rules, and constraints the agent must follow. -->\n")
	return b.String()
}
