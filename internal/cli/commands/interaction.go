package commands

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/stroops/sloop/internal/config"
)

type Interaction struct {
	Auto    bool
	NoInput bool
}

func ResolveInteraction(projectMode, globalMode string, autoFlag, noInput bool) Interaction {
	effective := projectMode
	if effective == "" {
		effective = globalMode
	}
	return Interaction{
		Auto:    autoFlag || effective == config.ModeAuto,
		NoInput: noInput,
	}
}

func (i Interaction) Confirm(prompt string, in io.Reader, out io.Writer) (bool, error) {
	if i.Auto {
		return true, nil
	}
	if i.NoInput {
		return false, fmt.Errorf("%s (refusing to prompt under --no-input)", prompt)
	}
	_, _ = fmt.Fprintf(out, "%s [y/N]: ", prompt)
	line, _ := bufio.NewReader(in).ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes", nil
}
