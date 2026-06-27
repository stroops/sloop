package tmux

import (
	"reflect"
	"testing"
)

func TestBuildPopupArgs(t *testing.T) {
	got := BuildPopupArgs("80%", "60%", "sloop ps")
	want := []string{"display-popup", "-w", "80%", "-h", "60%", "-E", "sloop ps"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestBuildBindArgs(t *testing.T) {
	got := BuildBindArgs("g", "sloop ps")
	if got[0] != "bind-key" || got[1] != "g" || got[len(got)-1] != "sloop ps" {
		t.Fatalf("got %v", got)
	}
}

func TestParseVersion(t *testing.T) {
	cases := map[string][2]int{
		"tmux 3.6b":       {3, 6},
		"tmux 3.2":        {3, 2},
		"tmux next-3.4":   {3, 4},
		"psmux 1.0":       {1, 0},
		"weird no number": {0, 0},
	}
	for in, want := range cases {
		maj, min := parseVersion(in)
		if maj != want[0] || min != want[1] {
			t.Fatalf("parseVersion(%q) = %d.%d, want %d.%d", in, maj, min, want[0], want[1])
		}
	}
}
