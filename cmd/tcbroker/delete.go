package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"tcbroker/pkg/manager"
	"tcbroker/pkg/state"
	"tcbroker/pkg/tc"
)

var (
	deleteAll    bool
	deleteDryRun bool
)

var deleteCmd = &cobra.Command{
	Use:     "delete [rule-name...]",
	Aliases: []string{"stop", "rm"},
	Short:   "Remove tcbroker-managed mirroring rules without needing the config file",
	Long: `Removes specific mirroring rules by name or all tcbroker-managed rules (--all).

Only TC filters installed by tcbroker are deleted. External TC filters and
pre-existing clsact qdiscs are left untouched.

Examples:
  sudo tcbroker delete web-traffic       # Remove a single rule
  sudo tcbroker delete --all             # Remove all tcbroker rules
  sudo tcbroker stop                     # Shorthand for 'delete --all'`,
	RunE: runDelete,
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolVar(&deleteAll, "all", false, "Delete all tcbroker-managed rules")
	deleteCmd.Flags().BoolVar(&deleteDryRun, "dry-run", false, "Preview deletions and tc commands without executing them")
}

func runDelete(cmd *cobra.Command, args []string) error {
	all := deleteAll
	// `tcbroker stop` with no args intuitively stops all managed rules.
	if cmd.CalledAs() == "stop" && len(args) == 0 {
		all = true
	}

	if all && len(args) > 0 {
		return fmt.Errorf("cannot specify both rule names and --all")
	}

	if !all && len(args) == 0 {
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

	if !deleteDryRun && os.Geteuid() != 0 {
		return fmt.Errorf("this command requires root privileges (use sudo, or pass --dry-run to preview)")
	}

	runner := tc.NewRunner(flagDebug, deleteDryRun)
	mgr := manager.New(runner, resolvedStatePath())

	res, err := mgr.Delete(args, all)
	if err != nil {
		return err
	}

	if len(res.DeletedFilters) == 0 {
		fmt.Println("No active tcbroker rules to remove.")
		return nil
	}

	if deleteDryRun {
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
