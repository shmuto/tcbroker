package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"tcbroker/pkg/manager"
	"tcbroker/pkg/state"
	"tcbroker/pkg/tc"
)

var (
	statusWatch    bool
	statusInterval time.Duration
	statusJSON     bool
	statusRaw      bool
	statusAll      bool
)

var statusCmd = &cobra.Command{
	Use:   "status [rule-name]",
	Short: "Show active mirroring rules, health, and traffic counters",
	Long: `Displays the current status and packet/byte counters for tcbroker-managed rules
without requiring the original config file.

Optionally pass a rule name to inspect per-filter details, or use --watch to
monitor live packet and byte rates.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVarP(&statusWatch, "watch", "w", false, "Continuously monitor status and live packet/byte rates")
	statusCmd.Flags().DurationVar(&statusInterval, "interval", 1*time.Second, "Refresh interval for --watch")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output status as JSON")
	statusCmd.Flags().BoolVar(&statusRaw, "raw", false, "Show raw `tc -s filter show` output for managed interfaces")
	statusCmd.Flags().BoolVar(&statusAll, "all", false, "Show all TC qdiscs on the system")
}

func runStatus(cmd *cobra.Command, args []string) error {
	runner := tc.NewRunner(flagDebug, false)

	if statusAll {
		out, err := runner.GetAllInterfaces()
		if err != nil {
			return err
		}
		fmt.Println("=== All TC Qdiscs on System ===")
		fmt.Print(out)
		return nil
	}

	if statusRaw {
		return printRawManagedTC(runner)
	}

	ruleFilter := ""
	if len(args) == 1 {
		ruleFilter = args[0]
	}

	mgr := manager.New(runner, resolvedStatePath())

	if statusWatch {
		if statusInterval <= 0 {
			return fmt.Errorf("--interval must be greater than 0")
		}
		return watchStatus(mgr, ruleFilter, statusInterval)
	}

	report, err := mgr.Status(ruleFilter)
	if err != nil {
		return err
	}

	if statusJSON {
		return printStatusJSON(report)
	}

	renderStatusReport(report, ruleFilter, nil, 0)
	return nil
}

func watchStatus(mgr *manager.Manager, ruleFilter string, interval time.Duration) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var prev *manager.StatusReport
	var prevTime time.Time

	for {
		now := time.Now()
		report, err := mgr.Status(ruleFilter)
		if err != nil {
			return err
		}

		if statusJSON {
			if err := printStatusJSON(report); err != nil {
				return err
			}
		} else {
			// Clear screen and move cursor home
			fmt.Print("\033[H\033[2J")
			fmt.Printf("tcbroker status (refresh %s, %s) - press Ctrl+C to exit\n\n",
				interval, now.Format("15:04:05"))
			var elapsed float64
			if prev != nil {
				elapsed = now.Sub(prevTime).Seconds()
			}
			renderStatusReport(report, ruleFilter, prev, elapsed)
		}

		prev = report
		prevTime = now

		select {
		case <-sigCh:
			fmt.Println()
			return nil
		case <-ticker.C:
		}
	}
}

func printStatusJSON(report *manager.StatusReport) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func renderStatusReport(report *manager.StatusReport, ruleFilter string, prev *manager.StatusReport, elapsedSec float64) {
	if len(report.Rules) == 0 {
		fmt.Println("No active tcbroker rules.")
		fmt.Println("Run `sudo tcbroker apply <config.yaml>` to start mirroring.")
		return
	}

	// Detailed single-rule view
	if ruleFilter != "" && len(report.Rules) == 1 {
		renderRuleDetail(report.Rules[0], prev, elapsedSec)
		return
	}

	prevRules := make(map[string]manager.RuleStatus)
	if prev != nil {
		for _, r := range prev.Rules {
			prevRules[r.Name] = r
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	if prev != nil && elapsedSec > 0 {
		_, _ = fmt.Fprintln(w, "RULE\tSTATUS\tSRC\tDIR\tDST\tFILTERS\tPACKETS\tBYTES\tPPS\tRATE\tDROPPED")
	} else {
		_, _ = fmt.Fprintln(w, "RULE\tSTATUS\tSRC\tDIR\tDST\tFILTERS\tPACKETS\tBYTES\tDROPPED")
	}

	for _, r := range report.Rules {
		dir := strings.Join(r.Hooks, ",")
		dsts := strings.Join(r.Destinations, ",")
		filtersCol := fmt.Sprintf("%d/%d", r.ActiveFilters, r.TotalFilters)

		if prev != nil && elapsedSec > 0 {
			var pps, bps float64
			if pr, ok := prevRules[r.Name]; ok {
				dp := r.Packets - pr.Packets
				db := r.Bytes - pr.Bytes
				if dp > 0 {
					pps = float64(dp) / elapsedSec
				}
				if db > 0 {
					bps = float64(db) / elapsedSec
				}
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%.1f\t%s\t%d\n",
				r.Name, r.Health, r.SrcIntf, dir, dsts, filtersCol,
				r.Packets, tc.FormatBytes(r.Bytes), pps, tc.FormatByteRate(bps), r.Dropped)
		} else {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%d\n",
				r.Name, r.Health, r.SrcIntf, dir, dsts, filtersCol,
				r.Packets, tc.FormatBytes(r.Bytes), r.Dropped)
		}
	}
	_ = w.Flush()

	if report.ConfigPath != "" {
		fmt.Printf("\nConfig: %s\n", report.ConfigPath)
	}
	if report.HasDrift() {
		hintCfg := "<config.yaml>"
		if report.ConfigPath != "" {
			hintCfg = report.ConfigPath
		}
		fmt.Printf("Warning: one or more filters are missing from kernel TC. Run `sudo tcbroker apply %s` to reconcile.\n", hintCfg)
	}
}

func renderRuleDetail(r manager.RuleStatus, prev *manager.StatusReport, elapsedSec float64) {
	fmt.Printf("Rule:    %s (%s)\n", r.Name, r.Health)
	fmt.Printf("Source:  %s (%s)\n", r.SrcIntf, strings.Join(r.Hooks, ", "))
	fmt.Printf("Targets: %s\n", strings.Join(r.Destinations, ", "))
	fmt.Printf("Totals:  %d packets, %s, %d dropped\n\n", r.Packets, tc.FormatBytes(r.Bytes), r.Dropped)

	prevFilters := make(map[string]manager.FilterStatus)
	if prev != nil && len(prev.Rules) == 1 {
		for _, f := range prev.Rules[0].Filters {
			key := fmt.Sprintf("%s:%d", f.Managed.Hook, f.Managed.Pref)
			prevFilters[key] = f
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	if prev != nil && elapsedSec > 0 {
		_, _ = fmt.Fprintln(w, "PREF\tSTATE\tHOOK\tDST\tMATCH\tREWRITE\tPACKETS\tBYTES\tPPS\tRATE\tDROPPED")
	} else {
		_, _ = fmt.Fprintln(w, "PREF\tSTATE\tHOOK\tDST\tMATCH\tREWRITE\tPACKETS\tBYTES\tDROPPED")
	}

	for _, fs := range r.Filters {
		st := "ACTIVE"
		if !fs.Present {
			st = "MISSING"
		}
		rw := "-"
		if !fs.Managed.Rewrite.IsEmpty() {
			rw = fs.Managed.Rewrite.Summary()
		}

		if prev != nil && elapsedSec > 0 {
			var pps, bps float64
			key := fmt.Sprintf("%s:%d", fs.Managed.Hook, fs.Managed.Pref)
			if pf, ok := prevFilters[key]; ok {
				if dp := fs.Packets - pf.Packets; dp > 0 {
					pps = float64(dp) / elapsedSec
				}
				if db := fs.Bytes - pf.Bytes; db > 0 {
					bps = float64(db) / elapsedSec
				}
			}
			_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%.1f\t%s\t%d\n",
				fs.Managed.Pref, st, fs.Managed.Hook, fs.Managed.DstIntf,
				fs.Managed.Filter.Summary(), rw,
				fs.Packets, tc.FormatBytes(fs.Bytes), pps, tc.FormatByteRate(bps), fs.Dropped)
		} else {
			_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%d\n",
				fs.Managed.Pref, st, fs.Managed.Hook, fs.Managed.DstIntf,
				fs.Managed.Filter.Summary(), rw,
				fs.Packets, tc.FormatBytes(fs.Bytes), fs.Dropped)
		}
	}
	_ = w.Flush()
}

func printRawManagedTC(runner *tc.Runner) error {
	st, err := state.Load(resolvedStatePath())
	if err != nil {
		return err
	}
	if len(st.Filters) == 0 {
		fmt.Println("No active tcbroker rules.")
		return nil
	}

	type ifaceHook struct{ src, hook string }
	seen := make(map[ifaceHook]bool)
	for _, f := range st.Filters {
		k := ifaceHook{src: f.SrcIntf, hook: f.Hook}
		if seen[k] {
			continue
		}
		seen[k] = true
		fmt.Printf("=== %s (%s) ===\n", f.SrcIntf, f.Hook)
		out, err := runner.ListFiltersWithStats(f.SrcIntf, f.Hook)
		if err != nil {
			return err
		}
		if strings.TrimSpace(out) == "" {
			fmt.Println("(no filters)")
		} else {
			fmt.Print(out)
		}
		fmt.Println()
	}
	return nil
}
