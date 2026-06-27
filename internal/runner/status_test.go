package runner

import "testing"

func TestClassifyStatus(t *testing.T) {
	cases := []struct {
		name string
		pane string
		want AgentStatus
	}{
		{"empty", "", StatusUnknown},
		{"blank lines", "\n\n   \n", StatusUnknown},
		{"claude approval menu", "Edit main.go?\n❯ 1. Yes\n  2. No", StatusWaiting},
		{"yes/no prompt", "Apply these changes? (y/n)", StatusWaiting},
		{"press enter", "Review complete.\nPress Enter to continue", StatusWaiting},
		{"waiting for your", "│ Waiting for your approval to edit main.go", StatusWaiting},
		{"esc to interrupt", "Editing files… (esc to interrupt)", StatusWorking},
		{"spinner", "⠹ Running tests", StatusWorking},
		{"tokens", "Thinking (1.2k tokens)", StatusWorking},
		{"idle prompt", "✓ Done. 12 passed.\n> ", StatusIdle},
		{"plain output", "Here is the summary of the file.", StatusIdle},
		{"waiting beats working", "esc to interrupt\nDo you want to proceed?", StatusWaiting},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyStatus(c.pane); got != c.want {
				t.Fatalf("ClassifyStatus(%q) = %v, want %v", c.pane, got, c.want)
			}
		})
	}
}

func TestAgentStatusString(t *testing.T) {
	for s, want := range map[AgentStatus]string{
		StatusUnknown: "unknown",
		StatusIdle:    "idle",
		StatusWorking: "working",
		StatusWaiting: "waiting",
	} {
		if got := s.String(); got != want {
			t.Fatalf("%d.String() = %q, want %q", s, got, want)
		}
	}
	if !StatusWaiting.NeedsAttention() || StatusIdle.NeedsAttention() {
		t.Fatal("NeedsAttention wrong")
	}
}
