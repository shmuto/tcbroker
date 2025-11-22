package tc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// FilterStats represents statistics for a single tc filter
type FilterStats struct {
	Protocol  string
	Priority  int
	Handle    string
	Chain     int
	MatchType string // "flower", "u32", etc.
	Matches   map[string]string
	Actions   []ActionStats
}

// ActionStats represents statistics for a filter action (e.g., mirred)
type ActionStats struct {
	Type         string // "mirred", etc.
	Operation    string // "Egress Mirror", "Egress Redirect", etc.
	TargetDev    string // Target device name
	Packets      int64
	Bytes        int64
	Dropped      int64
	Overlimits   int64
	Requeues     int64
	BacklogBytes int64
	BacklogPkts  int64
	Installed    string // "19 sec"
	Used         string // "19 sec"
}

// ParseFilterStats parses the output of `tc -s filter show` command
func ParseFilterStats(output string) ([]FilterStats, error) {
	if strings.TrimSpace(output) == "" {
		return []FilterStats{}, nil
	}

	var filters []FilterStats

	// Split by filter blocks - each filter starts with "filter protocol"
	lines := strings.Split(output, "\n")

	var currentFilter *FilterStats
	var currentAction *ActionStats
	inActionStats := false

	for _, line := range lines {
		line = strings.TrimRight(line, "\r\n")

		// New filter block - only count lines with "handle" as actual filter entries
		if strings.HasPrefix(line, "filter protocol") && strings.Contains(line, "handle") {
			// Save previous filter if exists
			if currentFilter != nil && currentAction != nil {
				currentFilter.Actions = append(currentFilter.Actions, *currentAction)
				currentAction = nil
			}
			if currentFilter != nil {
				filters = append(filters, *currentFilter)
			}

			// Parse filter header
			currentFilter = &FilterStats{
				Matches: make(map[string]string),
				Actions: []ActionStats{},
			}

			// Parse: filter protocol ip pref 49152 flower chain 0 handle 0x1
			parts := strings.Fields(line)
			for i := 0; i < len(parts); i++ {
				switch parts[i] {
				case "protocol":
					if i+1 < len(parts) {
						currentFilter.Protocol = parts[i+1]
					}
				case "pref":
					if i+1 < len(parts) {
						if pref, errConv := strconv.Atoi(parts[i+1]); errConv == nil {
							currentFilter.Priority = pref
						}
					}
				case "handle":
					if i+1 < len(parts) {
						currentFilter.Handle = parts[i+1]
					}
				case "chain":
					if i+1 < len(parts) {
						if chain, errConv := strconv.Atoi(parts[i+1]); errConv == nil {
							currentFilter.Chain = chain
						}
					}
				case "flower", "u32", "basic":
					currentFilter.MatchType = parts[i]
				}
			}
			continue
		}

		if currentFilter == nil {
			continue
		}

		// Match conditions (indented lines before action)
		if strings.HasPrefix(line, "  ") && !strings.Contains(line, "action") && !inActionStats {
			line = strings.TrimSpace(line)
			// Parse match conditions like "eth_type ipv4", "ip_proto icmp", "dst_port 80"
			if strings.Contains(line, " ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					key := parts[0]
					value := strings.Join(parts[1:], " ")
					currentFilter.Matches[key] = value
				}
			} else if line != "" && line != "not_in_hw" {
				currentFilter.Matches[line] = "true"
			}
			continue
		}

		// Action line: "action order 1: mirred (Egress Mirror to device veth1) pipe"
		if strings.Contains(line, "action order") {
			// Save previous action if exists
			if currentAction != nil {
				currentFilter.Actions = append(currentFilter.Actions, *currentAction)
			}

			currentAction = &ActionStats{}
			inActionStats = false

			// Parse mirred action
			if strings.Contains(line, "mirred") {
				currentAction.Type = "mirred"

				// Extract operation and target device
				// Example: "mirred (Egress Mirror to device veth1) pipe"
				re := regexp.MustCompile(`mirred \(([^)]+)\)`)
				if matches := re.FindStringSubmatch(line); len(matches) > 1 {
					opParts := strings.Fields(matches[1])
					// "Egress Mirror to device veth1"
					if len(opParts) >= 2 {
						currentAction.Operation = opParts[0] + " " + opParts[1] // "Egress Mirror"
					}
					if len(opParts) >= 4 && opParts[2] == "to" && opParts[3] == "device" && len(opParts) >= 5 {
						currentAction.TargetDev = opParts[4]
					}
				}
			}
			continue
		}

		// Metadata line: "index 1 ref 1 bind 1 installed 19 sec used 19 sec"
		if currentAction != nil && strings.Contains(line, "installed") && strings.Contains(line, "used") {
			parts := strings.Fields(strings.TrimSpace(line))
			for i := 0; i < len(parts); i++ {
				if parts[i] == "installed" && i+2 < len(parts) {
					currentAction.Installed = parts[i+1] + " " + parts[i+2]
				}
				if parts[i] == "used" && i+2 < len(parts) {
					currentAction.Used = parts[i+1] + " " + parts[i+2]
				}
			}
			continue
		}

		// Action statistics header
		if strings.Contains(line, "Action statistics:") {
			inActionStats = true
			continue
		}

		// Parse statistics: "Sent 840 bytes 10 pkt (dropped 0, overlimits 0 requeues 0)"
		if inActionStats && currentAction != nil && strings.Contains(line, "Sent") {
			line = strings.TrimSpace(line)

			// Extract: Sent 840 bytes 10 pkt
			re := regexp.MustCompile(`Sent (\d+) bytes (\d+) pkt`)
			if matches := re.FindStringSubmatch(line); len(matches) == 3 {
				if bytes, errConv := strconv.ParseInt(matches[1], 10, 64); errConv == nil {
					currentAction.Bytes = bytes
				}
				if pkts, errConv := strconv.ParseInt(matches[2], 10, 64); errConv == nil {
					currentAction.Packets = pkts
				}
			}

			// Extract: (dropped 0, overlimits 0 requeues 0)
			re = regexp.MustCompile(`dropped (\d+)`)
			if matches := re.FindStringSubmatch(line); len(matches) == 2 {
				if dropped, errConv := strconv.ParseInt(matches[1], 10, 64); errConv == nil {
					currentAction.Dropped = dropped
				}
			}

			re = regexp.MustCompile(`overlimits (\d+)`)
			if matches := re.FindStringSubmatch(line); len(matches) == 2 {
				if overlimits, errConv := strconv.ParseInt(matches[1], 10, 64); errConv == nil {
					currentAction.Overlimits = overlimits
				}
			}

			re = regexp.MustCompile(`requeues (\d+)`)
			if matches := re.FindStringSubmatch(line); len(matches) == 2 {
				if requeues, errConv := strconv.ParseInt(matches[1], 10, 64); errConv == nil {
					currentAction.Requeues = requeues
				}
			}
			continue
		}

		// Parse backlog: "backlog 0b 0p requeues 0"
		if inActionStats && currentAction != nil && strings.Contains(line, "backlog") {
			line = strings.TrimSpace(line)
			re := regexp.MustCompile(`backlog (\d+)b (\d+)p`)
			if matches := re.FindStringSubmatch(line); len(matches) == 3 {
				if bytes, errConv := strconv.ParseInt(matches[1], 10, 64); errConv == nil {
					currentAction.BacklogBytes = bytes
				}
				if pkts, errConv := strconv.ParseInt(matches[2], 10, 64); errConv == nil {
					currentAction.BacklogPkts = pkts
				}
			}
			inActionStats = false
			continue
		}
	}

	// Save last filter
	if currentFilter != nil {
		if currentAction != nil {
			currentFilter.Actions = append(currentFilter.Actions, *currentAction)
		}
		filters = append(filters, *currentFilter)
	}

	return filters, nil
}

// FormatBytes converts bytes to human-readable format
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// GetMatchDescription returns a human-readable description of the filter match
func (f *FilterStats) GetMatchDescription() string {
	parts := []string{}

	if proto, ok := f.Matches["ip_proto"]; ok {
		parts = append(parts, strings.ToUpper(proto))
	}

	if srcIP, ok := f.Matches["src_ip"]; ok {
		parts = append(parts, fmt.Sprintf("src=%s", srcIP))
	}

	if dstIP, ok := f.Matches["dst_ip"]; ok {
		parts = append(parts, fmt.Sprintf("dst=%s", dstIP))
	}

	if srcPort, ok := f.Matches["src_port"]; ok {
		parts = append(parts, fmt.Sprintf("sport=%s", srcPort))
	}

	if dstPort, ok := f.Matches["dst_port"]; ok {
		parts = append(parts, fmt.Sprintf("dport=%s", dstPort))
	}

	if len(parts) == 0 {
		return "ALL"
	}

	return strings.Join(parts, " ")
}
