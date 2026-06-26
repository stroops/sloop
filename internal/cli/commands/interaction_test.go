package commands

import (
	"strings"
	"testing"

	"github.com/stroops/sloop/internal/config"
)

func TestResolveInteractionPrecedence(t *testing.T) {
	// Flag wins.
	if !ResolveInteraction("", "", true, false).Auto {
		t.Fatal("flag should force auto")
	}
	// Project mode auto.
	if !ResolveInteraction(config.ModeAuto, "", false, false).Auto {
		t.Fatal("project mode auto should win")
	}
	// Global mode auto when project empty.
	if !ResolveInteraction("", config.ModeAuto, false, false).Auto {
		t.Fatal("global mode auto should apply")
	}
	// Default ask.
	if ResolveInteraction("", "", false, false).Auto {
		t.Fatal("default should not be auto")
	}
}

func TestConfirmAutoAndNoInput(t *testing.T) {
	ok, err := Interaction{Auto: true}.Confirm("go?", strings.NewReader(""), &strings.Builder{})
	if err != nil || !ok {
		t.Fatalf("auto should confirm true: ok=%v err=%v", ok, err)
	}
	if _, err := (Interaction{NoInput: true}).Confirm("go?", strings.NewReader(""), &strings.Builder{}); err == nil {
		t.Fatal("no-input should error instead of prompting")
	}
}

func TestConfirmReadsYes(t *testing.T) {
	ok, err := Interaction{}.Confirm("go?", strings.NewReader("y\n"), &strings.Builder{})
	if err != nil || !ok {
		t.Fatalf("want true on y: ok=%v err=%v", ok, err)
	}
	ok, _ = Interaction{}.Confirm("go?", strings.NewReader("n\n"), &strings.Builder{})
	if ok {
		t.Fatal("want false on n")
	}
}
