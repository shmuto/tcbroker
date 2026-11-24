package main

import (
	"fmt"
	"io"
	"os"

	"tcbroker/pkg/config"
)

func printInitUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  tcbroker init [output-file] [flags]

Writes a documented starter configuration file.
Defaults to ./tcbroker.yaml, or pass '-' to print to stdout.

Flags:
  -f, --force               Overwrite output file if it already exists
      --state-file string   Override state file path
      --debug               Print all tc commands executed
  -h, --help                Show help
`)
}

func cmdInit(_ string, args []string) error {
	var force bool
	fs := newFlagSet("init", printInitUsage)
	fs.BoolVar(&force, "force", false, "Overwrite output file if it already exists")
	fs.BoolVar(&force, "f", false, "Shorthand for --force")

	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 1 {
		return fmt.Errorf("expected at most 1 argument: [output-file] (run `tcbroker init --help` for usage)")
	}

	outPath := "tcbroker.yaml"
	if len(pos) == 1 {
		outPath = pos[0]
	}

	if outPath == "-" {
		fmt.Print(config.SampleConfigTemplate)
		return nil
	}

	if !force {
		if _, err := os.Stat(outPath); err == nil {
			return fmt.Errorf("file %q already exists (use --force to overwrite)", outPath)
		}
	}

	if err := os.WriteFile(outPath, []byte(config.SampleConfigTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outPath, err)
	}

	fmt.Printf("Created %s\n", outPath)
	fmt.Printf("Edit the interface names and run: sudo tcbroker apply %s\n", outPath)
	return nil
}
