package commands

import (
	"testing"

	"github.com/stroops/sloop/internal/tmux"
)

func row(name string, st tmux.AgentStatus) FleetRow {
	return FleetRow{Name: name, Status: st}
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
