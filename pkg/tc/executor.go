package tc

import (
	"bytes"
	"os/exec"
)

// CommandExecutor abstracts invoking the `tc` binary so unit tests can mock TC operations.
type CommandExecutor interface {
	Execute(args ...string) (stdout string, stderr string, err error)
}

// RealExecutor executes the system `tc` binary via os/exec.
type RealExecutor struct{}

// Execute invokes `tc <args...>`.
func (RealExecutor) Execute(args ...string) (string, string, error) {
	cmd := exec.Command("tc", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
