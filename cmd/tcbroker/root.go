package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"tcbroker/pkg/state"
)

var (
	flagStateFile string
	flagDebug     bool
)

// errHelpRequested signals that the user asked for help and we should exit 0.
var errHelpRequested = errors.New("help requested")

type command struct {
	name    string
	aliases []string
	short   string
	run     func(calledAs string, args []string) error
}

func commands() []command {
	return []command{
		{
			name:    "apply",
			aliases: []string{"start"},
			short:   "Idempotently apply mirroring rules from a YAML config file",
			run:     cmdApply,
		},
		{
			name:    "delete",
			aliases: []string{"stop", "rm"},
			short:   "Remove tcbroker-managed mirroring rules without needing the config file",
			run:     cmdDelete,
		},
		{
			name:  "status",
			short: "Show active mirroring rules, health, and traffic counters",
			run:   cmdStatus,
		},
		{
			name:  "validate",
			short: "Validate a configuration file without applying it",
			run:   cmdValidate,
		},
		{
			name:  "init",
			short: "Generate a starter tcbroker YAML configuration file",
			run:   cmdInit,
		},
		{
			name:    "serve",
			aliases: []string{"ui"},
			short:   "Start the embedded mini-NPB WebUI, REST API, and Prometheus /metrics server",
			run:     cmdServe,
		},
		{
			name:  "version",
			short: "Print tcbroker version",
			run:   cmdVersion,
		},
	}
}

func findCommand(name string) *command {
	for _, c := range commands() {
		if c.name == name {
			cmd := c
			return &cmd
		}
		for _, alias := range c.aliases {
			if alias == name {
				cmd := c
				return &cmd
			}
		}
	}
	return nil
}

func resolvedStatePath() string {
	return state.ResolveStatePath(flagStateFile)
}

// Execute parses top-level arguments and dispatches to the matching subcommand.
func Execute() {
	if err := runCLI(os.Args[1:]); err != nil {
		if errors.Is(err, errHelpRequested) {
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runCLI(args []string) error {
	// Reset global flags per invocation (useful for tests).
	flagStateFile = ""
	flagDebug = false

	// Parse leading global flags before the subcommand name.
	rootFS := newFlagSet("tcbroker", printRootUsage)
	if err := rootFS.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return errHelpRequested
		}
		return err
	}

	rest := rootFS.Args()
	if len(rest) == 0 {
		printRootUsage(os.Stdout)
		return nil
	}

	subName := rest[0]
	subArgs := rest[1:]

	if subName == "help" {
		if len(subArgs) == 0 {
			printRootUsage(os.Stdout)
			return nil
		}
		target := findCommand(subArgs[0])
		if target == nil {
			return fmt.Errorf("unknown command %q (run `tcbroker --help` for usage)", subArgs[0])
		}
		return target.run(target.name, []string{"--help"})
	}

	cmd := findCommand(subName)
	if cmd == nil {
		return fmt.Errorf("unknown command %q (run `tcbroker --help` for usage)", subName)
	}
	return cmd.run(subName, subArgs)
}

func printRootUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `tcbroker manages Linux Traffic Control (tc) packet mirroring rules declaratively.

It tracks its own TC filters in a state file and dedicated priority range so you can:
  - Apply configs idempotently with automatic rollback on error
  - Inspect status and real-time counters without passing the config file
  - Delete individual rules or stop everything without affecting other TC tools

Usage:
  tcbroker [global-flags] <command> [flags] [args]

Available Commands:
  apply       Idempotently apply mirroring rules from a YAML config file (alias: start)
  delete      Remove tcbroker-managed mirroring rules (aliases: stop, rm)
  status      Show active mirroring rules, health, and traffic counters
  validate    Validate a configuration file without applying it
  init        Generate a starter tcbroker YAML configuration file
  version     Print tcbroker version
  help        Show help for a command

Global Flags:
      --state-file string   Path to state file (default %q or $%s)
      --debug               Print all tc commands executed
  -h, --help                Show help

Use "tcbroker <command> --help" for more information about a command.
`, state.DefaultStatePath, state.EnvStateFile)
}

func newFlagSet(name string, usage func(io.Writer)) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&flagStateFile, "state-file", flagStateFile,
		fmt.Sprintf("Path to state file (default %q or $%s)", state.DefaultStatePath, state.EnvStateFile))
	fs.BoolVar(&flagDebug, "debug", flagDebug, "Print all tc commands executed")
	fs.Usage = func() {
		usage(os.Stdout)
	}
	return fs
}

// parseFlags parses flags regardless of whether they appear before or after positional arguments,
// matching POSIX/GNU CLI conventions (e.g. `tcbroker apply config.yaml --dry-run`).
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	reordered := reorderArgs(fs, args)
	if err := fs.Parse(reordered); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, errHelpRequested
		}
		return nil, err
	}
	return fs.Args(), nil
}

// isBoolFlag checks whether a registered flag implements flag's internal IsBoolFlag interface.
func isBoolFlag(f *flag.Flag) bool {
	type boolFlagger interface {
		IsBoolFlag() bool
	}
	bf, ok := f.Value.(boolFlagger)
	return ok && bf.IsBoolFlag()
}

// reorderArgs moves flag tokens ahead of positional arguments while preserving `--` and single `-`.
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	var flags []string
	var positionals []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Stop reordering after `--`
		if arg == "--" {
			positionals = append(positionals, args[i:]...)
			break
		}

		// Bare `-` (e.g. stdout) or non-flag token is positional
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			positionals = append(positionals, arg)
			continue
		}

		// Strip leading `-` or `--`
		trimmed := strings.TrimPrefix(arg, "--")
		if trimmed == arg {
			trimmed = strings.TrimPrefix(arg, "-")
		}

		// Handle `--flag=value` or `-f=value`
		if strings.Contains(trimmed, "=") {
			flags = append(flags, arg)
			continue
		}

		flags = append(flags, arg)
		if trimmed == "h" || trimmed == "help" {
			continue
		}

		// If the flag expects a value (not a bool flag), attach the next argument as its value.
		if f := fs.Lookup(trimmed); f != nil && !isBoolFlag(f) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	return append(flags, positionals...)
}
