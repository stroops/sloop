// Package skills manages the workspace skills lockfile (.sloop/skills.lock),
// which records each URL-imported skill and its source so `sloop skills update`
// can re-fetch reproducibly and a team gets the same skills from one source.
//
// Only skills imported from a source (sloop skills add <url>) are locked;
// locally authored skills (sloop skills new) are not re-fetchable and stay out.
package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// LockVersion is the current skills.lock schema version.
const LockVersion = 1

const lockName = "skills.lock"

// Entry is one locked skill: a stable name, the source it was fetched from, and
// the content hash + fetch time so update can report unchanged vs. refreshed.
type Entry struct {
	Name    string `yaml:"name"`
	Source  string `yaml:"source"`
	SHA256  string `yaml:"sha256,omitempty"`
	Updated string `yaml:"updated,omitempty"`
}

// Lock is the parsed skills.lock for one workspace.
type Lock struct {
	Version int     `yaml:"version"`
	Skills  []Entry `yaml:"skills"`
}

// LockPath returns the lockfile path for a given .sloop directory.
func LockPath(sloopDir string) string { return filepath.Join(sloopDir, lockName) }

// Load reads the lockfile. A missing file is an empty lock, not an error.
func Load(sloopDir string) (*Lock, error) {
	b, err := os.ReadFile(LockPath(sloopDir))
	if os.IsNotExist(err) {
		return &Lock{Version: LockVersion}, nil
	}
	if err != nil {
		return nil, err
	}
	var l Lock
	if err := yaml.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	if l.Version == 0 {
		l.Version = LockVersion
	}
	return &l, nil
}

// Save writes the lockfile (sorted by name for stable diffs).
func (l *Lock) Save(sloopDir string) error {
	if l.Version == 0 {
		l.Version = LockVersion
	}
	sort.Slice(l.Skills, func(i, j int) bool { return l.Skills[i].Name < l.Skills[j].Name })
	if err := os.MkdirAll(sloopDir, 0o700); err != nil {
		return err
	}
	b, err := yaml.Marshal(l)
	if err != nil {
		return err
	}
	return os.WriteFile(LockPath(sloopDir), b, 0o600)
}

// Get returns the entry for name, if present.
func (l *Lock) Get(name string) (Entry, bool) {
	for _, e := range l.Skills {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

// Upsert replaces the entry with the same name or appends a new one.
func (l *Lock) Upsert(e Entry) {
	for i := range l.Skills {
		if l.Skills[i].Name == e.Name {
			l.Skills[i] = e
			return
		}
	}
	l.Skills = append(l.Skills, e)
}

// Hash returns the hex SHA-256 of content, used to detect changes on update.
func Hash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
