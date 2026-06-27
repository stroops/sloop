package runner

import (
	"os"
	"os/exec"
)

type Spec struct {
	Dir     string
	Command string
	Args    []string
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
	return cmd
}

type ExecRunner struct{}

func (ExecRunner) Launch(s Spec) error {
	return BuildExecCmd(s).Run()
}
