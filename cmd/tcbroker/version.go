package main

import (
	"fmt"
	"io"
)

// version can be overridden at build time via -ldflags "-X main.version=v2.0.0".
var version = "2.0.0"

func printVersionUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  tcbroker version

Prints the tcbroker version.
`)
}

func cmdVersion(_ string, args []string) error {
	fs := newFlagSet("version", printVersionUsage)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return fmt.Errorf("version takes no arguments")
	}
	fmt.Printf("tcbroker %s\n", version)
	return nil
}
