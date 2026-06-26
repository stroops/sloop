package sync

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stroops/sloop/internal/adapter"
	"github.com/stroops/sloop/internal/profile"
)

func Assemble(sloopDir string, p profile.Profile) (string, error) {
	var b strings.Builder

	// 1. all context/*.md, alphabetical
	contextDir := filepath.Join(sloopDir, "context")
	names, err := markdownFiles(contextDir)
	if err != nil {
		return "", err
	}
	for _, name := range names {
		if err := appendFile(&b, contextDir, name); err != nil {
			return "", err
		}
	}

	// 2. selected skills, in listed order
	for _, name := range p.Skills {
		if err := appendFile(&b, filepath.Join(sloopDir, "skills"), name); err != nil {
			return "", err
		}
	}

	// 3. selected vault, in listed order
	for _, name := range p.Vault {
		if err := appendFile(&b, filepath.Join(sloopDir, "vault"), name); err != nil {
			return "", err
		}
	}

	return b.String(), nil
}

func markdownFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func appendFile(b *strings.Builder, dir, name string) error {
	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	b.WriteString("## " + name + "\n\n")
	b.Write(content)
	b.WriteString("\n\n")
	return nil
}

func WriteNativeFiles(root string, m adapter.Manifest, assembled string) ([]string, error) {
	rendered := m.Render(assembled)
	var written []string
	for rel, content := range rendered {
		dest := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
			return nil, err
		}
		written = append(written, dest)
	}
	return written, nil
}
