package commands

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroops/sloop/internal/workspace"
)

// fetchURL is a seam so skill import is testable without a network.
var fetchURL = func(url string) ([]byte, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // cap at 1 MiB
}

// rawURL rewrites a GitHub "blob" URL to its raw form; other URLs pass through.
func rawURL(u string) string {
	const gh = "https://github.com/"
	if rest, ok := strings.CutPrefix(u, gh); ok {
		if i := strings.Index(rest, "/blob/"); i >= 0 {
			return "https://raw.githubusercontent.com/" + rest[:i] + "/" + rest[i+len("/blob/"):]
		}
	}
	return u
}

// skillNameFromURL derives a skill name from a URL's filename (without .md).
func skillNameFromURL(u string) string {
	base := path.Base(u)
	if i := strings.IndexAny(base, "?#"); i >= 0 {
		base = base[:i]
	}
	return strings.TrimSuffix(base, ".md")
}

// RunSkillAdd fetches a markdown skill from a URL (GitHub blob URLs are rewritten
// to raw), saves it under .sloop/skills, and ensures it is linked into tools.
func RunSkillAdd(startDir, url, name string) (string, []string, error) {
	ws, err := workspace.Resolve(startDir)
	if err != nil {
		return "", nil, err
	}
	if name == "" {
		name = skillNameFromURL(url)
	}
	if name == "" {
		return "", nil, fmt.Errorf("could not derive a skill name from %q; pass --name", url)
	}
	dst := filepath.Join(ws.SloopDir(), "skills", name+".md")
	if _, err := os.Stat(dst); err == nil {
		return "", nil, fmt.Errorf("skill %q already exists at %s", name, dst)
	}
	body, err := fetchURL(rawURL(url))
	if err != nil {
		return "", nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return "", nil, fmt.Errorf("fetched empty content from %s", url)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(dst, body, 0o600); err != nil {
		return "", nil, err
	}
	return dst, ensureSkillsLinked(ws), nil
}

var skillAddName string

var skillAddCmd = &cobra.Command{
	Use:     "add <url>",
	Aliases: []string{"import"},
	Short:   "Import a skill from a URL or GitHub into .sloop/skills",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		dst, linked, err := RunSkillAdd(cwd, args[0], skillAddName)
		if err != nil {
			return err
		}
		cmd.Printf("imported %s\n", dst)
		if len(linked) > 0 {
			cmd.Printf("available to: %s\n", strings.Join(linked, ", "))
		} else {
			cmd.Printf("run `sloop sync` to link skills into your tools\n")
		}
		return nil
	},
}
