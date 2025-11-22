package tc

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Runner is responsible for executing tc commands.
type Runner struct {
	Debug  bool
	DryRun bool
}

// NewRunner creates a new Runner.
func NewRunner(debug, dryRun bool) *Runner {
	return &Runner{
		Debug:  debug,
		DryRun: dryRun,
	}
}

// Run executes a tc command with the given arguments.
func (r *Runner) Run(args ...string) (string, string, error) {
	commandString := fmt.Sprintf("tc %s", strings.Join(args, " "))

	if r.DryRun || r.Debug {
		fmt.Println(commandString)
	}

	if r.DryRun {
		return "", "", nil
	}

	cmd := exec.Command("tc", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}
