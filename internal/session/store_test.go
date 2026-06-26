package session

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "sloop.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRegisterWorkspaceIdempotent(t *testing.T) {
	s := newTestStore(t)
	w1, err := s.RegisterWorkspace("backend", "/tmp/backend")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	w2, err := s.RegisterWorkspace("backend", "/tmp/backend")
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if w1.ID != w2.ID {
		t.Fatalf("expected same id, got %d and %d", w1.ID, w2.ID)
	}
	got, err := s.WorkspaceByName("backend")
	if err != nil {
		t.Fatalf("by name: %v", err)
	}
	if got.Path != "/tmp/backend" {
		t.Fatalf("want /tmp/backend, got %s", got.Path)
	}
}

func TestRecordAndListSessions(t *testing.T) {
	s := newTestStore(t)
	w, _ := s.RegisterWorkspace("backend", "/tmp/backend")
	id, err := s.RecordSession(Session{
		WorkspaceID: w.ID, Tool: "claude", Cwd: "/tmp/backend", StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := s.EndSession(id, time.Now()); err != nil {
		t.Fatalf("end: %v", err)
	}
	list, err := s.ListSessions(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Tool != "claude" || list[0].EndedAt == nil {
		t.Fatalf("unexpected sessions: %+v", list)
	}
}
