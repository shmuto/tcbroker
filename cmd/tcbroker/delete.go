package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"tcbroker/pkg/manager"
	"tcbroker/pkg/state"
	"tcbroker/pkg/tc"
)

func printDeleteUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  tcbroker delete <rule-name...> [flags]
  tcbroker delete --all [flags]
  tcbroker stop [rule-name...] [flags]

Removes specific mirroring rules by name or all tcbroker-managed rules (--all).
Running 'tcbroker stop' with no arguments removes all managed rules.

Only TC filters installed by tcbroker are deleted. External TC filters and
pre-existing clsact qdiscs are left untouched.

Flags:
      --all                 Delete all tcbroker-managed rules
      --dry-run             Preview deletions and tc commands without executing them
      --state-file string   Override state file path
      --debug               Print all tc commands executed
  -h, --help                Show help
`)
}

func cmdDelete(calledAs string, args []string) error {
	var all, dryRun bool
	fs := newFlagSet("delete", printDeleteUsage)
	fs.BoolVar(&all, "all", false, "Delete all tcbroker-managed rules")
	fs.BoolVar(&dryRun, "dry-run", false, "Preview deletions and tc commands without executing them")

	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}

	// `tcbroker stop` with no args intuitively stops all managed rules.
	if calledAs == "stop" && len(pos) == 0 {
		all = true
	}

	if all && len(pos) > 0 {
		return fmt.Errorf("cannot specify both rule names and --all")
	}

	if !all && len(pos) == 0 {
		st, err := state.Load(resolvedStatePath())
		if err != nil {
			return err
		}
		if len(st.Filters) == 0 {
			fmt.Println("No active tcbroker rules.")
			return nil
		}
		return fmt.Errorf("specify rule name(s) to delete or use --all (active rules: %s)", strings.Join(st.RuleNames(), ", "))
	}

	if !dryRun && os.Geteuid() != 0 {
		return fmt.Errorf("this command requires root privileges (use sudo, or pass --dry-run to preview)")
	}

	runner := tc.NewRunner(flagDebug, dryRun)
	mgr := manager.New(runner, resolvedStatePath())

	res, err := mgr.Delete(pos, all)
	if err != nil {
		return err
	}

	if len(res.DeletedFilters) == 0 {
		fmt.Println("No active tcbroker rules to remove.")
		return nil
	}

	if dryRun {
		fmt.Printf("[dry-run] Would delete %d filter(s) from rule(s): %s\n",
			len(res.DeletedFilters), strings.Join(res.DeletedRules, ", "))
		return nil
	}

	fmt.Printf("Deleted %d filter(s) from rule(s): %s\n",
		len(res.DeletedFilters), strings.Join(res.DeletedRules, ", "))
	if len(res.RemovedQdiscs) > 0 {
		fmt.Printf("Removed unused clsact qdisc on: %s\n", strings.Join(res.RemovedQdiscs, ", "))
	}
	if res.RemainingRules > 0 {
		fmt.Printf("%d rule(s) still active.\n", res.RemainingRules)
	}
	return nil
}
