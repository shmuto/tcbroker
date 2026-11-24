package main

import (
	"fmt"
	"io"
	"net"
	"strings"

	"tcbroker/pkg/config"
)

func printValidateUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  tcbroker validate <config-file> [flags]

Reads and strictly validates the YAML configuration file, checking for
unknown fields, syntax errors, logical errors (such as missing proto for port
filters or self-mirror loops), and optionally verifying that interfaces exist.

Flags:
      --check-interfaces    Verify that specified network interfaces exist on the system
      --state-file string   Override state file path
      --debug               Print all tc commands executed
  -h, --help                Show help
`)
}

func cmdValidate(_ string, args []string) error {
	var checkInterfaces bool
	fs := newFlagSet("validate", printValidateUsage)
	fs.BoolVar(&checkInterfaces, "check-interfaces", false, "Verify that specified network interfaces exist on the system")

	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("expected exactly 1 argument: <config-file> (run `tcbroker validate --help` for usage)")
	}
	configFile := pos[0]

	fmt.Printf("Validating %s...\n", configFile)

	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}

	fmt.Printf("OK: syntax and rules are valid (%d rule(s))\n", len(cfg.Rules))

	for i, rule := range cfg.Rules {
		fmt.Printf("\n  Rule #%d: %s\n", i+1, rule.Name)
		fmt.Printf("    Source:    %s (%s)\n", rule.Src, rule.EffectiveDirection())

		var dstDescs []string
		for _, dst := range rule.Dst {
			desc := dst.Intf
			if rw := rule.EffectiveRewrite(dst); !rw.IsEmpty() {
				desc += fmt.Sprintf(" [rewrite: %s]", rw.Summary())
			}
			dstDescs = append(dstDescs, desc)
		}
		fmt.Printf("    Targets:   %s\n", strings.Join(dstDescs, ", "))

		var filterDescs []string
		for _, f := range rule.EffectiveFilters() {
			filterDescs = append(filterDescs, f.Summary())
		}
		fmt.Printf("    Filters:   %s\n", strings.Join(filterDescs, "; "))
	}

	warnings := cfg.Warnings()
	if len(warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, w := range warnings {
			fmt.Printf("  ! %s\n", w)
		}
	}

	if checkInterfaces {
		fmt.Println("\nChecking network interfaces...")
		var missing []string
		for _, ifaceName := range cfg.AllInterfaces() {
			if _, errIface := net.InterfaceByName(ifaceName); errIface != nil {
				fmt.Printf("  MISSING  %s (%v)\n", ifaceName, errIface)
				missing = append(missing, ifaceName)
			} else {
				fmt.Printf("  OK       %s\n", ifaceName)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("missing interface(s): %s", strings.Join(missing, ", "))
		}
	}

	fmt.Println("\nConfiguration is ready to apply.")
	return nil
}
