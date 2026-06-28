package runner

import (
	"os"
	"os/exec"
	"sort"
)

type Spec struct {
	Dir     string
	Command string
	Args    []string
	// Env is extra environment for the launched command (e.g. CLAUDE_CONFIG_DIR
	// to select a second account); empty leaves the inherited environment alone.
	Env map[string]string
}

type Runner interface {
	Launch(Spec) error
}

func BuildExecCmd(s Spec) *exec.Cmd {
	cmd := exec.Command(s.Command, s.Args...)
	cmd.Dir = s.Dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(s.Env) > 0 {
		keys := make([]string, 0, len(s.Env))
		for k := range s.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		env := os.Environ()
		for _, k := range keys {
			env = append(env, k+"="+s.Env[k])
		}
		cmd.Env = env
	}
	return cmd
}

type ExecRunner struct{}

func (ExecRunner) Launch(s Spec) error {
	return BuildExecCmd(s).Run()
}
