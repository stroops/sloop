package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/skills"
	"github.com/stroops/sloop/internal/workspace"
)

// SkillUpdateOutcome is the per-skill result of an update pass.
type SkillUpdateOutcome struct {
	Name    string
	Updated bool   // content changed and was rewritten
	Err     error  // fetch/write failure (others still proceed)
	Source  string // for messaging
}

// SkillUpdateResult summarizes an update run.
type SkillUpdateResult struct {
	Outcomes []SkillUpdateOutcome
	Linked   []string // tools relinked when something changed
}

// RunSkillUpdate re-fetches locked skills from their recorded sources and
// rewrites any whose content changed. With no names it updates every locked
// skill; otherwise only the named ones. A missing/empty lock is reported, not
// an error.
func RunSkillUpdate(startDir string, names []string) (*SkillUpdateResult, error) {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return nil, err
	}
	lock, err := skills.Load(ws.SloopDir())
	if err != nil {
		return nil, err
	}

	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	for _, n := range names {
		if _, ok := lock.Get(n); !ok {
			return nil, fmt.Errorf("skill %q is not in skills.lock; import it with `sloop skills add <url>`", n)
		}
	}

	res := &SkillUpdateResult{}
	changed := false
	for _, e := range lock.Skills {
		if len(want) > 0 && !want[e.Name] {
			continue
		}
		out := SkillUpdateOutcome{Name: e.Name, Source: e.Source}
		body, err := fetchURL(rawURL(e.Source))
		if err != nil {
			out.Err = err
			res.Outcomes = append(res.Outcomes, out)
			continue
		}
		if len(strings.TrimSpace(string(body))) == 0 {
			out.Err = fmt.Errorf("fetched empty content from %s", e.Source)
			res.Outcomes = append(res.Outcomes, out)
			continue
		}
		if h := skills.Hash(body); h != e.SHA256 {
			dst := filepath.Join(ws.SloopDir(), "skills", e.Name+".md")
			if err := os.WriteFile(dst, body, 0o600); err != nil {
				out.Err = err
				res.Outcomes = append(res.Outcomes, out)
				continue
			}
			e.SHA256 = h
			e.Updated = time.Now().UTC().Format(time.RFC3339)
			lock.Upsert(e)
			out.Updated = true
			changed = true
		}
		res.Outcomes = append(res.Outcomes, out)
	}

	if changed {
		if err := lock.Save(ws.SloopDir()); err != nil {
			return nil, err
		}
		res.Linked = ensureSkillsLinked(ws)
	}
	return res, nil
}

var skillUpdateCmd = &cobra.Command{
	Use:     "update [name...]",
	Aliases: []string{"up"},
	Short:   "Re-fetch locked skills from their sources (.sloop/skills.lock)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		res, err := RunSkillUpdate(cwd, args)
		if err != nil {
			return err
		}
		if len(res.Outcomes) == 0 {
			cmd.Println("no locked skills; import one with `sloop skills add <url>`")
			return nil
		}
		updated, failed := 0, 0
		for _, o := range res.Outcomes {
			switch {
			case o.Err != nil:
				failed++
				cmd.Printf("✗ %s: %v\n", o.Name, o.Err)
			case o.Updated:
				updated++
				cmd.Printf("↑ %s updated\n", o.Name)
			default:
				cmd.Printf("= %s unchanged\n", o.Name)
			}
		}
		cmd.Printf("%d updated, %d unchanged, %d failed\n",
			updated, len(res.Outcomes)-updated-failed, failed)
		if len(res.Linked) > 0 {
			cmd.Printf("relinked into: %s\n", strings.Join(res.Linked, ", "))
		}
		if failed > 0 {
			return fmt.Errorf("%d skill(s) failed to update", failed)
		}
		return nil
	},
}
