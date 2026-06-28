package runner

import (
	"slices"
	"testing"
)

func TestBuildExecCmdSetsDirAndArgs(t *testing.T) {
	cmd := BuildExecCmd(Spec{Dir: "/tmp/backend", Command: "claude", Args: []string{"--resume"}})
	if cmd.Dir != "/tmp/backend" {
		t.Fatalf("want dir /tmp/backend, got %s", cmd.Dir)
	}
	// cmd.Args[0] is the command name, followed by the args.
	if len(cmd.Args) != 2 || cmd.Args[0] != "claude" || cmd.Args[1] != "--resume" {
		t.Fatalf("unexpected args: %v", cmd.Args)
	}
}

func TestBuildExecCmdEnv(t *testing.T) {
	cmd := BuildExecCmd(Spec{Command: "claude", Env: map[string]string{"CLAUDE_CONFIG_DIR": "/x"}})
	if !slices.Contains(cmd.Env, "CLAUDE_CONFIG_DIR=/x") {
		t.Fatalf("env not injected: %v", cmd.Env)
	}
	// No Env → leave cmd.Env nil (inherit parent), unchanged from before.
	if BuildExecCmd(Spec{Command: "claude"}).Env != nil {
		t.Fatal("no Env should leave cmd.Env nil")
	}
}
