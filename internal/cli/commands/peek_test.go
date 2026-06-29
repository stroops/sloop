package commands

import (
	"testing"

	"github.com/stroops/sloop/internal/tmux"
)

func row(name string, st tmux.AgentStatus) FleetRow {
	return FleetRow{Name: name, Status: st}
}

func TestExcludeSession(t *testing.T) {
	rows := []FleetRow{
		row("ws__claude", tmux.StatusWorking),
		row("ws__gemini", tmux.StatusWaiting),
	}
	got := excludeSession(rows, "ws__claude")
	if len(got) != 1 || got[0].Name != "ws__gemini" {
		t.Fatalf("expected only ws__gemini, got %+v", got)
	}
	// empty self → no filtering
	all := excludeSession(rows, "")
	if len(all) != 2 {
		t.Fatalf("empty self should keep all rows, got %+v", all)
	}
	// unknown self → no filtering
	same := excludeSession(rows, "other")
	if len(same) != 2 {
		t.Fatalf("non-matching self should keep all rows, got %+v", same)
	}
}

func TestSoleWaiting(t *testing.T) {
	cases := []struct {
		name     string
		rows     []FleetRow
		wantName string
		wantOK   bool
	}{
		{"none waiting", []FleetRow{row("a__claude", tmux.StatusWorking)}, "", false},
		{"one waiting", []FleetRow{
			row("a__claude", tmux.StatusWorking),
			row("b__claude", tmux.StatusWaiting),
		}, "b__claude", true},
		{"many waiting", []FleetRow{
			row("a__claude", tmux.StatusWaiting),
			row("b__claude", tmux.StatusWaiting),
		}, "", false},
		{"empty", nil, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotName, gotOK := soleWaiting(c.rows)
			if gotName != c.wantName || gotOK != c.wantOK {
				t.Fatalf("got (%q,%v) want (%q,%v)", gotName, gotOK, c.wantName, c.wantOK)
			}
		})
	}
}
