package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

// migrations are applied in order; the current schema version is the slice
// length, tracked in SQLite's built-in PRAGMA user_version (no migrations
// table, no framework). To evolve the schema, append one entry — never edit a
// shipped one. Each must be safe to re-run (IF NOT EXISTS) so existing DBs that
// predate user_version tracking migrate cleanly.
var migrations = []string{
	// v1: initial schema.
	`CREATE TABLE IF NOT EXISTS workspaces (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT UNIQUE NOT NULL,
  path       TEXT UNIQUE NOT NULL,
  created_at TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL REFERENCES workspaces(id),
  tool         TEXT NOT NULL,
  profile      TEXT,
  cwd          TEXT,
  tmux_session TEXT,
  started_at   TIMESTAMP NOT NULL,
  ended_at     TIMESTAMP
);`,
}

// dsnPragmas hardens the DB for concurrent cross-repo writes: WAL allows
// readers and a writer at once, busy_timeout retries instead of erroring on a
// lock, and foreign_keys enforces referential integrity.
const dsnPragmas = "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+dsnPragmas)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// migrate applies any migrations newer than the DB's user_version, each in its
// own transaction, then bumps user_version.
func migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	for i := v; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			_ = tx.Rollback()
			return err
		}
		// user_version can't be parameterized; i+1 is a trusted int.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

type Workspace struct {
	ID        int64
	Name      string
	Path      string
	CreatedAt time.Time
}

func (s *Store) RegisterWorkspace(name, path string) (*Workspace, error) {
	_, err := s.db.Exec(
		`INSERT INTO workspaces(name, path, created_at) VALUES(?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET name=excluded.name`,
		name, path, time.Now(),
	)
	if err != nil {
		return nil, err
	}
	return s.workspaceByPath(path)
}

func (s *Store) workspaceByPath(path string) (*Workspace, error) {
	var w Workspace
	err := s.db.QueryRow(
		`SELECT id, name, path, created_at FROM workspaces WHERE path = ?`, path,
	).Scan(&w.ID, &w.Name, &w.Path, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (s *Store) WorkspaceByName(name string) (*Workspace, error) {
	var w Workspace
	err := s.db.QueryRow(
		`SELECT id, name, path, created_at FROM workspaces WHERE name = ?`, name,
	).Scan(&w.ID, &w.Name, &w.Path, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (s *Store) ListWorkspaces() ([]Workspace, error) {
	rows, err := s.db.Query(`SELECT id, name, path, created_at FROM workspaces ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Workspace
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Path, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// PruneWorkspaces removes workspace registrations whose paths no longer exist on
// disk (e.g. temp directories from sloop run/init in $TMPDIR). Also removes the
// associated session records. Returns the names of deleted workspaces.
func (s *Store) PruneWorkspaces() ([]string, error) {
	wss, err := s.ListWorkspaces()
	if err != nil {
		return nil, err
	}
	var pruned []string
	for _, ws := range wss {
		if _, err := os.Stat(ws.Path); err == nil {
			continue
		}
		if _, err := s.db.Exec(`DELETE FROM sessions WHERE workspace_id = ?`, ws.ID); err != nil {
			return pruned, err
		}
		if _, err := s.db.Exec(`DELETE FROM workspaces WHERE id = ?`, ws.ID); err != nil {
			return pruned, err
		}
		pruned = append(pruned, ws.Name)
	}
	return pruned, nil
}

type Session struct {
	ID          int64
	WorkspaceID int64
	Tool        string
	Profile     string
	Cwd         string
	TmuxSession string
	StartedAt   time.Time
	EndedAt     *time.Time
}

func (s *Store) RecordSession(sess Session) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO sessions(workspace_id, tool, profile, cwd, tmux_session, started_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		sess.WorkspaceID, sess.Tool, sess.Profile, sess.Cwd, sess.TmuxSession, sess.StartedAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) EndSession(id int64, ended time.Time) error {
	_, err := s.db.Exec(`UPDATE sessions SET ended_at = ? WHERE id = ?`, ended, id)
	return err
}

func (s *Store) ListSessions(limit int) ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, workspace_id, tool, COALESCE(profile,''), COALESCE(cwd,''),
		        COALESCE(tmux_session,''), started_at, ended_at
		 FROM sessions ORDER BY started_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.Tool, &s.Profile, &s.Cwd,
			&s.TmuxSession, &s.StartedAt, &s.EndedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
