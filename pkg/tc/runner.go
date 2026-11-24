package tc

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Runner is responsible for executing tc commands.
type Runner struct {
	Debug    bool
	DryRun   bool
	Executor CommandExecutor
	Output   io.Writer
}

// NewRunner creates a Runner backed by the real `tc` binary.
func NewRunner(debug, dryRun bool) *Runner {
	return &Runner{
		Debug:    debug,
		DryRun:   dryRun,
		Executor: RealExecutor{},
		Output:   os.Stdout,
	}
}

// NewRunnerWithExecutor creates a Runner with a custom CommandExecutor (useful for testing).
func NewRunnerWithExecutor(exec CommandExecutor, debug, dryRun bool, out io.Writer) *Runner {
	if out == nil {
		out = io.Discard
	}
	return &Runner{
		Debug:    debug,
		DryRun:   dryRun,
		Executor: exec,
		Output:   out,
	}
}

func (r *Runner) executor() CommandExecutor {
	if r.Executor == nil {
		return RealExecutor{}
	}
	return r.Executor
}

func (r *Runner) out() io.Writer {
	if r.Output == nil {
		return os.Stdout
	}
	return r.Output
}

// RunWrite executes a mutating tc command. In DryRun mode it logs the command without executing it.
func (r *Runner) RunWrite(args ...string) (string, string, error) {
	commandString := fmt.Sprintf("tc %s", strings.Join(args, " "))
	if r.DryRun || r.Debug {
		_, _ = fmt.Fprintln(r.out(), commandString)
	}
	if r.DryRun {
		return "", "", nil
	}
	return r.executor().Execute(args...)
}

// RunRead executes a read-only tc command (always executed even in DryRun mode so diffs can be inspected).
func (r *Runner) RunRead(args ...string) (string, string, error) {
	if r.Debug {
		_, _ = fmt.Fprintf(r.out(), "tc %s\n", strings.Join(args, " "))
	}
	return r.executor().Execute(args...)
}
