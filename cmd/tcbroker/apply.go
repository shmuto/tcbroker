package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"tcbroker/pkg/config"
	"tcbroker/pkg/manager"
	"tcbroker/pkg/state"
	"tcbroker/pkg/tc"
)

var applyDryRun bool

var applyCmd = &cobra.Command{
	Use:     "apply <config-file>",
	Aliases: []string{"start"},
	Short:   "Idempotently apply mirroring rules from a YAML config file",
	Long: `Reads and validates the YAML configuration file, compares it against currently
managed TC filters, and applies only the necessary changes.

Existing unchanged filters keep their counters intact. External TC filters and
pre-existing clsact qdiscs are never modified or deleted. If any filter fails
to apply, newly added filters are automatically rolled back.`,
	Args: cobra.ExactArgs(1),
	RunE: runApply,
}

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Preview planned changes and tc commands without applying them")
}

func runApply(cmd *cobra.Command, args []string) error {
	configFile := args[0]

	if !applyDryRun && os.Geteuid() != 0 {
		return fmt.Errorf("this command requires root privileges (use sudo, or pass --dry-run to preview)")
	}

	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}

	for _, w := range cfg.Warnings() {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	// Verify all referenced interfaces exist before mutating TC state.
	if !applyDryRun {
		var missing []string
		for _, iface := range cfg.AllInterfaces() {
			if _, errIface := net.InterfaceByName(iface); errIface != nil {
				missing = append(missing, iface)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("network interface(s) not found: %s (check with `ip link show`)", strings.Join(missing, ", "))
		}
	}

	absPath, err := filepath.Abs(configFile)
	if err != nil {
		absPath = configFile
	}

	runner := tc.NewRunner(flagDebug, applyDryRun)
	mgr := manager.New(runner, resolvedStatePath())

	if applyDryRun {
		plan, err := mgr.PlanApply(cfg, absPath)
		if err != nil {
			return err
		}
		printPlanSummary(plan, true)
		if plan.HasChanges() {
			fmt.Println("\nPlanned tc commands:")
			if _, err := mgr.Apply(cfg, absPath); err != nil {
				return err
			}
		}
		return nil
	}

	res, err := mgr.Apply(cfg, absPath)
	if err != nil {
		return err
	}

	if !res.Plan.HasChanges() && res.Plan.DriftedCount == 0 {
		fmt.Printf("Already up to date (%d filter(s) active across %d rule(s); no changes needed).\n",
			len(res.Plan.Kept), len(cfg.Rules))
		return nil
	}

	printPlanSummary(res.Plan, false)
	if len(res.CreatedQdiscs) > 0 {
		fmt.Printf("Created clsact qdisc on: %s\n", strings.Join(res.CreatedQdiscs, ", "))
	}
	if len(res.RemovedQdiscs) > 0 {
		fmt.Printf("Removed unused clsact qdisc on: %s\n", strings.Join(res.RemovedQdiscs, ", "))
	}
	fmt.Printf("Applied %s (%d active rule(s)). Run `tcbroker status` to inspect.\n", configFile, len(cfg.Rules))
	return nil
}

func printPlanSummary(plan *manager.ApplyPlan, dryRun bool) {
	prefix := ""
	if dryRun {
		prefix = "[dry-run] "
	}
	if !plan.HasChanges() {
		fmt.Printf("%sNo changes needed (%d filter(s) already match).\n", prefix, len(plan.Kept))
		return
	}

	fmt.Printf("%sPlan: +%d to add, -%d to delete, =%d unchanged",
		prefix, len(plan.ToAdd), len(plan.ToDelete), len(plan.Kept))
	if plan.DriftedCount > 0 {
		fmt.Printf(" (%d drifted filter(s) will be restored)", plan.DriftedCount)
	}
	fmt.Println()

	for _, f := range plan.ToAdd {
		fmt.Printf("  + %s\n", formatManagedFilterLine(f))
	}
	for _, f := range plan.ToDelete {
		fmt.Printf("  - %s\n", formatManagedFilterLine(f))
	}
}

func formatManagedFilterLine(f state.ManagedFilter) string {
	line := fmt.Sprintf("[%s] %s (%s) -> %s  pref=%d  match=%s",
		f.RuleName, f.SrcIntf, f.Hook, f.DstIntf, f.Pref, f.Filter.Summary())
	if !f.Rewrite.IsEmpty() {
		line += fmt.Sprintf("  rewrite=%s", f.Rewrite.Summary())
	}
	return line
}
