package tmux

import (
	"testing"

	"github.com/stroops/sloop/internal/adapter"
)

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
			if got := ClassifyStatus(c.pane, adapter.Manifest{}); got != c.want {
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

func TestParseAnswers(t *testing.T) {
	yn := ParseAnswers("Apply changes to main.go? (y/n)")
	if len(yn) != 2 || yn[0].Key != "y" || yn[1].Key != "n" {
		t.Fatalf("y/n: %+v", yn)
	}
	menu := ParseAnswers("Edit main.go?\n❯ 1. Yes\n  2. No, keep it\n  3. Always allow")
	if len(menu) != 3 || menu[0].Key != "1" || menu[0].Label != "Yes" || menu[2].Key != "3" {
		t.Fatalf("menu: %+v", menu)
	}
	cont := ParseAnswers("Done.\nPress Enter to continue")
	if len(cont) != 1 || cont[0].Key != "" {
		t.Fatalf("continue: %+v", cont)
	}
	if a := ParseAnswers("just some output, no prompt"); a != nil {
		t.Fatalf("none expected, got %+v", a)
	}
}

func TestPromptLine(t *testing.T) {
	if p := PromptLine("blah\nApply changes to main.go?\n(y/n)"); p != "Apply changes to main.go?" {
		t.Fatalf("question: %q", p)
	}
	if p := PromptLine("Choose:\n  1. Yes\n  2. No"); p != "Choose:" {
		t.Fatalf("above-choice: %q", p)
	}
}

func TestAffirmativeAnswer(t *testing.T) {
	a, ok := AffirmativeAnswer([]Answer{{Key: "1", Label: "Yes"}, {Key: "2", Label: "No"}})
	if !ok || a.Key != "1" {
		t.Fatalf("numbered yes: %+v %v", a, ok)
	}
	a, ok = AffirmativeAnswer([]Answer{{Key: "y", Label: "Yes"}, {Key: "n", Label: "No"}})
	if !ok || a.Key != "y" {
		t.Fatalf("yn: %+v %v", a, ok)
	}
	if _, ok := AffirmativeAnswer([]Answer{{Key: "3", Label: "Cancel"}}); ok {
		t.Fatal("no affirmative expected")
	}
}
