package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"tcbroker/pkg/manager"
	"tcbroker/pkg/state"
	"tcbroker/pkg/tc"
)

func printStatusUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  tcbroker status [rule-name] [flags]

Displays the current status and packet/byte counters for tcbroker-managed rules
without requiring the original config file.

Optionally pass a rule name to inspect per-filter details, or use --watch to
monitor live packet and byte rates.

Flags:
  -w, --watch               Continuously monitor status and live packet/byte rates
      --interval duration   Refresh interval for --watch (default 1s)
      --json                Output status as JSON
      --raw                 Show raw 'tc -s filter show' output for managed interfaces
      --all                 Show all TC qdiscs on the system
      --state-file string   Override state file path
      --debug               Print all tc commands executed
  -h, --help                Show help
`)
}

func cmdStatus(_ string, args []string) error {
	var (
		watch    bool
		interval time.Duration
		asJSON   bool
		raw      bool
		all      bool
	)

	fs := newFlagSet("status", printStatusUsage)
	fs.BoolVar(&watch, "watch", false, "Continuously monitor status and live packet/byte rates")
	fs.BoolVar(&watch, "w", false, "Shorthand for --watch")
	fs.DurationVar(&interval, "interval", 1*time.Second, "Refresh interval for --watch")
	fs.BoolVar(&asJSON, "json", false, "Output status as JSON")
	fs.BoolVar(&raw, "raw", false, "Show raw tc -s filter show output for managed interfaces")
	fs.BoolVar(&all, "all", false, "Show all TC qdiscs on the system")

	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 1 {
		return fmt.Errorf("expected at most 1 argument: [rule-name] (run `tcbroker status --help` for usage)")
	}

	runner := tc.NewRunner(flagDebug, false)

	if all {
		out, err := runner.GetAllInterfaces()
		if err != nil {
			return err
		}
		fmt.Println("=== All TC Qdiscs on System ===")
		fmt.Print(out)
		return nil
	}

	if raw {
		return printRawManagedTC(runner)
	}

	ruleFilter := ""
	if len(pos) == 1 {
		ruleFilter = pos[0]
	}

	mgr := manager.New(runner, resolvedStatePath())

	if watch {
		if interval <= 0 {
			return fmt.Errorf("--interval must be greater than 0")
		}
		return watchStatus(mgr, ruleFilter, interval, asJSON)
	}

	report, err := mgr.Status(ruleFilter)
	if err != nil {
		return err
	}

	if asJSON {
		return printStatusJSON(report)
	}

	renderStatusReport(report, ruleFilter, nil, 0)
	return nil
}

func watchStatus(mgr *manager.Manager, ruleFilter string, interval time.Duration, asJSON bool) error {
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

		if asJSON {
			if err := printStatusJSON(report); err != nil {
				return err
			}
		} else {
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

	formatPort := func(intf string) string {
		if p, ok := report.Ports[intf]; ok && p.Label != "" {
			return fmt.Sprintf("%s(%s)", p.Label, intf)
		}
		return intf
	}

	if ruleFilter != "" && len(report.Rules) == 1 {
		renderRuleDetail(report.Rules[0], formatPort, prev, elapsedSec)
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
		dstLabels := make([]string, 0, len(r.Destinations))
		for _, d := range r.Destinations {
			dstLabels = append(dstLabels, formatPort(d))
		}
		dsts := strings.Join(dstLabels, ",")
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
				r.Name, r.Health, formatPort(r.SrcIntf), dir, dsts, filtersCol,
				r.Packets, tc.FormatBytes(r.Bytes), pps, tc.FormatByteRate(bps), r.Dropped)
		} else {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%d\n",
				r.Name, r.Health, formatPort(r.SrcIntf), dir, dsts, filtersCol,
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

func renderRuleDetail(r manager.RuleStatus, formatPort func(string) string, prev *manager.StatusReport, elapsedSec float64) {
	dstLabels := make([]string, 0, len(r.Destinations))
	for _, d := range r.Destinations {
		dstLabels = append(dstLabels, formatPort(d))
	}
	fmt.Printf("Rule:    %s (%s)\n", r.Name, r.Health)
	fmt.Printf("Source:  %s (%s)\n", formatPort(r.SrcIntf), strings.Join(r.Hooks, ", "))
	fmt.Printf("Targets: %s\n", strings.Join(dstLabels, ", "))
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
		_, _ = fmt.Fprintln(w, "PREF\tSTATE\tHOOK\tDST\tMATCH\tACTIONS\tPACKETS\tBYTES\tPPS\tRATE\tDROPPED")
	} else {
		_, _ = fmt.Fprintln(w, "PREF\tSTATE\tHOOK\tDST\tMATCH\tACTIONS\tPACKETS\tBYTES\tDROPPED")
	}

	for _, fs := range r.Filters {
		st := "ACTIVE"
		if !fs.Present {
			st = "MISSING"
		}
		var acts []string
		if !fs.Managed.RateLimit.IsEmpty() {
			acts = append(acts, fs.Managed.RateLimit.Summary())
		}
		if !fs.Managed.VLAN.IsEmpty() {
			acts = append(acts, fs.Managed.VLAN.Summary())
		}
		if !fs.Managed.Rewrite.IsEmpty() {
			acts = append(acts, fs.Managed.Rewrite.Summary())
		}
		actCol := "-"
		if len(acts) > 0 {
			actCol = strings.Join(acts, " ")
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
				fs.Managed.Pref, st, fs.Managed.Hook, formatPort(fs.Managed.DstIntf),
				fs.Managed.Filter.Summary(), actCol,
				fs.Packets, tc.FormatBytes(fs.Bytes), pps, tc.FormatByteRate(bps), fs.Dropped)
		} else {
			_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%d\n",
				fs.Managed.Pref, st, fs.Managed.Hook, formatPort(fs.Managed.DstIntf),
				fs.Managed.Filter.Summary(), actCol,
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
