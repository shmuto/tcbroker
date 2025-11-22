package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"tcbroker/pkg/config"
	"tcbroker/pkg/filter"
	"tcbroker/pkg/tc"
)

var (
	showAll     bool
	showStats   bool
	showSummary bool
)

var statusCmd = &cobra.Command{
	Use:   "status [config-file]",
	Short: "Shows the current status of packet mirroring.",
	Long: `Displays the current tc configuration including active qdiscs and filters.
If a config file is provided, shows status for interfaces in the config.
Otherwise, shows all tc rules on the system.`,
	Args: cobra.MaximumNArgs(1),
	Run:  status,
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVar(&showAll, "all", false, "Show all tc rules on the system (ignores config file)")
	statusCmd.Flags().BoolVar(&showStats, "stats", false, "Show statistics (packet counts, byte counts)")
	statusCmd.Flags().BoolVar(&showSummary, "summary", false, "Show summarized statistics (parsed and formatted)")
}

func status(cmd *cobra.Command, args []string) {
	runner := tc.NewRunner(false, false)

	// If --all flag is set, show all tc rules on the system
	if showAll {
		fmt.Println("=== All TC Rules on System ===")
		allQdiscs, err := runner.GetAllInterfaces()
		if err != nil {
			fmt.Printf("Error listing qdiscs: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(allQdiscs)
		return
	}

	// If no config file is provided and --all is not set, show error
	if len(args) == 0 {
		fmt.Println("Error: Please provide a config file or use --all flag to show all tc rules.")
		fmt.Println("Usage: tcbroker status [config-file] or tcbroker status --all")
		os.Exit(1)
	}

	// Load the configuration
	configFile := args[0]
	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Printf("Error loading config file: %v\n", err)
		os.Exit(1)
	}

	// If --summary flag is set, show simple per-rule statistics
	if showSummary {
		fmt.Printf("%-30s  %-20s  %-20s  %10s  %s\n", "Name", "SrcIntf", "DstIntf", "Packets", "Bytes")
		for _, rule := range cfg.Rules {
			totalPackets, totalBytes := getRuleStats(runner, rule)
			fmt.Printf("%-30s  %-20s  %-20s  %10d  %s\n", rule.Name, rule.SrcIntf, rule.DstIntf, totalPackets, tc.FormatBytes(totalBytes))
		}
		return
	}

	fmt.Printf("=== TC Status for Configuration: %s ===\n\n", configFile)

	// Collect unique source interfaces from rules
	srcInterfaceSet := make(map[string]bool)
	for _, rule := range cfg.Rules {
		srcInterfaceSet[rule.SrcIntf] = true
	}

	// Show status for each source interface
	for srcIntf := range srcInterfaceSet {
		fmt.Printf("Interface: %s (direction: ingress)\n", srcIntf)
		fmt.Println(strings.Repeat("-", 60))

		// Check if interface has clsact qdisc
		hasClsact, err := runner.HasClsactQdisc(srcIntf)
		if err != nil {
			fmt.Printf("  Error checking qdisc: %v\n\n", err)
			continue
		}

		if !hasClsact {
			fmt.Printf("  Status: No clsact qdisc found (mirroring not active)\n\n")
			continue
		}

		fmt.Printf("  Status: Active (clsact qdisc present)\n\n")

		// Show qdiscs (only clsact)
		fmt.Printf("  Qdiscs:\n")
		qdiscs, err := runner.ListQdiscs(srcIntf)
		if err != nil {
			fmt.Printf("    Error: %v\n", err)
		} else {
			found := false
			for _, line := range strings.Split(strings.TrimSpace(qdiscs), "\n") {
				if line != "" && strings.Contains(line, "clsact") {
					fmt.Printf("    %s\n", line)
					found = true
				}
			}
			if !found {
				fmt.Printf("    (no clsact qdisc)\n")
			}
		}
		fmt.Println()

		// Show filters for ingress direction
		hook := "ingress"
		fmt.Printf("  Filters (%s):\n", hook)

		if showSummary {
			// Parse and show summarized statistics
			output, err := runner.ListFiltersWithStats(srcIntf, hook)
			if err != nil {
				fmt.Printf("    Error: %v\n", err)
				fmt.Println()
				continue
			}

			filters, parseErr := tc.ParseFilterStats(output)
			if parseErr != nil {
				fmt.Printf("    Error parsing filter stats: %v\n", parseErr)
				fmt.Println()
				continue
			}

			if len(filters) == 0 {
				fmt.Printf("    (no filters)\n")
			} else {
				var totalPackets, totalBytes int64
				for _, filter := range filters {
					desc := filter.GetMatchDescription()
					fmt.Printf("    %-20s", desc)

					if len(filter.Actions) > 0 {
						action := filter.Actions[0]
						fmt.Printf("→ %-8s  ", action.TargetDev)
						fmt.Printf("Packets: %-8d  Bytes: %-12s", action.Packets, tc.FormatBytes(action.Bytes))
						if action.Dropped > 0 {
							fmt.Printf("  Dropped: %d", action.Dropped)
						}
						totalPackets += action.Packets
						totalBytes += action.Bytes
					}
					fmt.Println()
				}

				if len(filters) > 1 {
					fmt.Printf("\n    %-20s  %-8s  Packets: %-8d  Bytes: %-12s\n",
						"TOTAL", "", totalPackets, tc.FormatBytes(totalBytes))
				}
			}
			fmt.Println()
		} else {
			// Show raw tc output
			var filters string
			if showStats {
				filters, err = runner.ListFiltersWithStats(srcIntf, hook)
			} else {
				filters, err = runner.ListFilters(srcIntf, hook)
			}

			if err != nil {
				fmt.Printf("    Error: %v\n", err)
			} else if strings.TrimSpace(filters) == "" {
				fmt.Printf("    (no filters)\n")
			} else {
				for _, line := range strings.Split(strings.TrimSpace(filters), "\n") {
					if line != "" {
						fmt.Printf("    %s\n", line)
					}
				}
			}
			fmt.Println()
		}

		fmt.Println()
	}
}

// getRuleStats retrieves statistics for a specific rule by matching tc filters
func getRuleStats(runner *tc.Runner, rule config.Rule) (int64, int64) {
	var totalPackets, totalBytes int64

	// Query filters for this rule's source interface
	output, err := runner.ListFiltersWithStats(rule.SrcIntf, "ingress")
	if err != nil {
		return 0, 0
	}

	tcFilters, parseErr := tc.ParseFilterStats(output)
	if parseErr != nil {
		return 0, 0
	}

	// For each filter in the rule's config
	for _, ruleFilter := range rule.Filters {
		// Try to match with tc filters
		for _, tcFilter := range tcFilters {
			if matchesFilter(tcFilter, ruleFilter, rule.DstIntf) {
				// Sum up the action statistics for the matching target device only
				// to avoid double counting when there are multiple actions (e.g., skbmod + mirred)
				for _, action := range tcFilter.Actions {
					if action.TargetDev == rule.DstIntf {
						totalPackets += action.Packets
						totalBytes += action.Bytes
					}
				}
			}
		}
	}

	return totalPackets, totalBytes
}

// matchesFilter checks if a tc filter matches a rule filter configuration
func matchesFilter(tcFilter tc.FilterStats, ruleFilter filter.Filter, dstIntf string) bool {
	// Check ip_proto
	if ruleFilter.IPProto != "" {
		if proto, ok := tcFilter.Matches["ip_proto"]; !ok || proto != ruleFilter.IPProto {
			return false
		}
	}

	// Check src_ip
	if ruleFilter.SrcIP != "" {
		if srcIP, ok := tcFilter.Matches["src_ip"]; !ok || srcIP != ruleFilter.SrcIP {
			return false
		}
	}

	// Check dst_ip
	if ruleFilter.DstIP != "" {
		if dstIP, ok := tcFilter.Matches["dst_ip"]; !ok || dstIP != ruleFilter.DstIP {
			return false
		}
	}

	// Check src_port
	if ruleFilter.SrcPort != 0 {
		if srcPort, ok := tcFilter.Matches["src_port"]; !ok || srcPort != strconv.Itoa(ruleFilter.SrcPort) {
			return false
		}
	}

	// Check dst_port
	if ruleFilter.DstPort != 0 {
		if dstPort, ok := tcFilter.Matches["dst_port"]; !ok || dstPort != strconv.Itoa(ruleFilter.DstPort) {
			return false
		}
	}

	// Check destination interface (target device)
	// Need to check all actions since there might be multiple actions (e.g., skbmod + mirred)
	if dstIntf != "" {
		found := false
		for _, action := range tcFilter.Actions {
			if action.TargetDev == dstIntf {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
