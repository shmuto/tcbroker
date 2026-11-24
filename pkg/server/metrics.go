package server

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"tcbroker/pkg/manager"
)

// WritePrometheusMetrics formats a StatusReport in standard Prometheus text exposition format (0.0.4)
// without requiring any third-party client libraries.
func WritePrometheusMetrics(w io.Writer, report *manager.StatusReport) {
	_, _ = fmt.Fprintln(w, "# HELP tcbroker_active_rules_total Total number of rules managed by tcbroker.")
	_, _ = fmt.Fprintln(w, "# TYPE tcbroker_active_rules_total gauge")
	_, _ = fmt.Fprintf(w, "tcbroker_active_rules_total %d\n", len(report.Rules))

	drifted := 0
	for _, r := range report.Rules {
		drifted += r.TotalFilters - r.ActiveFilters
	}
	_, _ = fmt.Fprintln(w, "# HELP tcbroker_drifted_filters_total Number of managed TC filters missing from kernel.")
	_, _ = fmt.Fprintln(w, "# TYPE tcbroker_drifted_filters_total gauge")
	_, _ = fmt.Fprintf(w, "tcbroker_drifted_filters_total %d\n", drifted)

	_, _ = fmt.Fprintln(w, "# HELP tcbroker_rule_health Rule health status (1=ACTIVE, 0.5=PARTIAL, 0=MISSING).")
	_, _ = fmt.Fprintln(w, "# TYPE tcbroker_rule_health gauge")
	for _, r := range report.Rules {
		val := 0.0
		switch r.Health {
		case manager.HealthActive:
			val = 1.0
		case manager.HealthPartial:
			val = 0.5
		}
		_, _ = fmt.Fprintf(w, "tcbroker_rule_health{rule=%q,src=%q} %g\n",
			escapeProm(r.Name), escapeProm(r.SrcIntf), val)
	}

	_, _ = fmt.Fprintln(w, "# HELP tcbroker_rule_packets_total Total mirrored packets per rule.")
	_, _ = fmt.Fprintln(w, "# TYPE tcbroker_rule_packets_total counter")
	for _, r := range report.Rules {
		_, _ = fmt.Fprintf(w, "tcbroker_rule_packets_total{rule=%q,src=%q} %d\n",
			escapeProm(r.Name), escapeProm(r.SrcIntf), r.Packets)
	}

	_, _ = fmt.Fprintln(w, "# HELP tcbroker_rule_bytes_total Total mirrored bytes per rule.")
	_, _ = fmt.Fprintln(w, "# TYPE tcbroker_rule_bytes_total counter")
	for _, r := range report.Rules {
		_, _ = fmt.Fprintf(w, "tcbroker_rule_bytes_total{rule=%q,src=%q} %d\n",
			escapeProm(r.Name), escapeProm(r.SrcIntf), r.Bytes)
	}

	_, _ = fmt.Fprintln(w, "# HELP tcbroker_rule_dropped_total Total dropped packets per rule.")
	_, _ = fmt.Fprintln(w, "# TYPE tcbroker_rule_dropped_total counter")
	for _, r := range report.Rules {
		_, _ = fmt.Fprintf(w, "tcbroker_rule_dropped_total{rule=%q,src=%q} %d\n",
			escapeProm(r.Name), escapeProm(r.SrcIntf), r.Dropped)
	}

	_, _ = fmt.Fprintln(w, "# HELP tcbroker_filter_present Whether an individual managed TC filter is present in kernel (1 or 0).")
	_, _ = fmt.Fprintln(w, "# TYPE tcbroker_filter_present gauge")
	for _, r := range report.Rules {
		for _, f := range r.Filters {
			present := 0
			if f.Present {
				present = 1
			}
			_, _ = fmt.Fprintf(w, "tcbroker_filter_present{rule=%q,src=%q,hook=%q,dst=%q,pref=%q} %d\n",
				escapeProm(r.Name), escapeProm(f.Managed.SrcIntf), escapeProm(f.Managed.Hook),
				escapeProm(f.Managed.DstIntf), strconv.Itoa(f.Managed.Pref), present)
		}
	}

	_, _ = fmt.Fprintln(w, "# HELP tcbroker_filter_packets_total Mirrored packets for an individual managed TC filter.")
	_, _ = fmt.Fprintln(w, "# TYPE tcbroker_filter_packets_total counter")
	for _, r := range report.Rules {
		for _, f := range r.Filters {
			_, _ = fmt.Fprintf(w, "tcbroker_filter_packets_total{rule=%q,src=%q,hook=%q,dst=%q,pref=%q} %d\n",
				escapeProm(r.Name), escapeProm(f.Managed.SrcIntf), escapeProm(f.Managed.Hook),
				escapeProm(f.Managed.DstIntf), strconv.Itoa(f.Managed.Pref), f.Packets)
		}
	}

	_, _ = fmt.Fprintln(w, "# HELP tcbroker_filter_bytes_total Mirrored bytes for an individual managed TC filter.")
	_, _ = fmt.Fprintln(w, "# TYPE tcbroker_filter_bytes_total counter")
	for _, r := range report.Rules {
		for _, f := range r.Filters {
			_, _ = fmt.Fprintf(w, "tcbroker_filter_bytes_total{rule=%q,src=%q,hook=%q,dst=%q,pref=%q} %d\n",
				escapeProm(r.Name), escapeProm(f.Managed.SrcIntf), escapeProm(f.Managed.Hook),
				escapeProm(f.Managed.DstIntf), strconv.Itoa(f.Managed.Pref), f.Bytes)
		}
	}
}

func escapeProm(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
