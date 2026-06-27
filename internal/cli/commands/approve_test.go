package commands

import (
	"bytes"
	"testing"

	"github.com/stroops/sloop/internal/tmux"
)

func TestAnswerHint(t *testing.T) {
	if h := answerHint([]tmux.Answer{{Key: "y", Label: "Yes"}, {Key: "n", Label: "No"}}); h != "[y]Yes [n]No" {
		t.Fatalf("yn hint = %q", h)
	}
	if h := answerHint([]tmux.Answer{{Key: "", Label: "continue"}}); h != "[⏎]continue" {
		t.Fatalf("continue hint = %q", h)
	}
	if h := answerHint(nil); h != "" {
		t.Fatalf("empty hint = %q", h)
	}
}

func TestMatchAnswer(t *testing.T) {
	r := FleetRow{Answers: []tmux.Answer{{Key: "y", Label: "Yes"}, {Key: "2", Label: "No"}}}
	if a, ok := matchAnswer(r, 'y'); !ok || a.Label != "Yes" {
		t.Fatalf("match y: %+v %v", a, ok)
	}
	if _, ok := matchAnswer(r, '9'); ok {
		t.Fatal("9 should not match")
	}
}

func TestBottomLineWaitingShowsAnswers(t *testing.T) {
	r := FleetRow{
		Status:  tmux.StatusWaiting,
		Prompt:  "Apply changes?",
		Answers: []tmux.Answer{{Key: "y", Label: "Yes"}, {Key: "n", Label: "No"}},
	}
	b := bottomLine(r)
	if b == "" || !bytes.Contains([]byte(b), []byte("answer: [y]Yes")) {
		t.Fatalf("bottomLine = %q", b)
	}
}
