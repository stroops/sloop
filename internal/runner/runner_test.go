package runner

import "testing"

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
