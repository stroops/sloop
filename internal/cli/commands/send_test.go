package commands

import "testing"

func TestResolveTarget(t *testing.T) {
	rows := []FleetRow{
		{Workspace: "api", Tool: "claude", Name: "api__claude"},
		{Workspace: "web", Tool: "claude", Name: "web__claude"},
		{Workspace: "web", Tool: "cursor", Name: "web__cursor"},
	}

	if _, err := resolveTarget(nil, "1"); err == nil {
		t.Fatal("expected error on empty fleet")
	}

	r, err := resolveTarget(rows, "1")
	if err != nil || r.Name != "api__claude" {
		t.Fatalf("by number: %v / %q", err, r.Name)
	}
	if _, err := resolveTarget(rows, "9"); err == nil {
		t.Fatal("expected out-of-range error")
	}

	r, err = resolveTarget(rows, "web__cursor")
	if err != nil || r.Name != "web__cursor" {
		t.Fatalf("by session name: %v / %q", err, r.Name)
	}

	r, err = resolveTarget(rows, "api")
	if err != nil || r.Name != "api__claude" {
		t.Fatalf("by unique workspace: %v / %q", err, r.Name)
	}

	if _, err := resolveTarget(rows, "web"); err == nil {
		t.Fatal("expected ambiguous error for workspace with two sessions")
	}
	if _, err := resolveTarget(rows, "nope"); err == nil {
		t.Fatal("expected unknown-target error")
	}
}
