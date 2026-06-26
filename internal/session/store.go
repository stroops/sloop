package session

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS workspaces (
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
);`

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
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
	defer rows.Close()
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
	defer rows.Close()
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
