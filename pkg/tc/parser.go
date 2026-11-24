package tc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reMirred     = regexp.MustCompile(`mirred \(([^)]+)\)`)
	reSentStats  = regexp.MustCompile(`Sent (\d+) bytes (\d+) pkt`)
	reDropped    = regexp.MustCompile(`dropped (\d+)`)
	reOverlimits = regexp.MustCompile(`overlimits (\d+)`)
	reRequeues   = regexp.MustCompile(`requeues (\d+)`)
	reBacklog    = regexp.MustCompile(`backlog (\d+)b (\d+)p`)
)

// FilterStats represents statistics for a single tc filter.
type FilterStats struct {
	Protocol  string
	Priority  int
	Handle    string
	Chain     int
	MatchType string // "flower", "u32", etc.
	Matches   map[string]string
	Actions   []ActionStats
}

// ActionStats represents statistics for a filter action (e.g., mirred, skbmod, pedit, csum).
type ActionStats struct {
	Type         string // "mirred", "skbmod", "pedit", "csum", etc.
	Operation    string // "Egress Mirror", "Egress Redirect", etc.
	TargetDev    string // Target device name (for mirred)
	Packets      int64
	Bytes        int64
	Dropped      int64
	Overlimits   int64
	Requeues     int64
	BacklogBytes int64
	BacklogPkts  int64
	Installed    string
	Used         string
}

// ParseFilterStats parses the output of `tc [-s] filter show` command.
func ParseFilterStats(output string) ([]FilterStats, error) {
	if strings.TrimSpace(output) == "" {
		return []FilterStats{}, nil
	}

	var filters []FilterStats
	lines := strings.Split(output, "\n")

	var currentFilter *FilterStats
	var currentAction *ActionStats
	inActionStats := false

	for _, line := range lines {
		line = strings.TrimRight(line, "\r\n")

		// New filter block - only count lines with "handle" as concrete filter instances
		if strings.HasPrefix(line, "filter protocol") && strings.Contains(line, "handle") {
			if currentFilter != nil && currentAction != nil {
				currentFilter.Actions = append(currentFilter.Actions, *currentAction)
				currentAction = nil
			}
			if currentFilter != nil {
				filters = append(filters, *currentFilter)
			}

			currentFilter = &FilterStats{
				Matches: make(map[string]string),
				Actions: []ActionStats{},
			}
			inActionStats = false

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
				case "flower", "u32", "basic", "bpf", "matchall":
					currentFilter.MatchType = parts[i]
				}
			}
			continue
		}

		if currentFilter == nil {
			continue
		}

		// Action line: e.g. "\taction order 1: mirred (Egress Mirror to device veth1) continue"
		if strings.Contains(line, "action order") {
			if currentAction != nil {
				currentFilter.Actions = append(currentFilter.Actions, *currentAction)
			}

			currentAction = &ActionStats{}
			inActionStats = false

			switch {
			case strings.Contains(line, "mirred"):
				currentAction.Type = "mirred"
				if matches := reMirred.FindStringSubmatch(line); len(matches) > 1 {
					opParts := strings.Fields(matches[1])
					if len(opParts) >= 2 {
						currentAction.Operation = opParts[0] + " " + opParts[1]
					}
					if len(opParts) >= 5 && opParts[2] == "to" && opParts[3] == "device" {
						currentAction.TargetDev = opParts[4]
					}
				}
			case strings.Contains(line, "skbmod"):
				currentAction.Type = "skbmod"
			case strings.Contains(line, "pedit"):
				currentAction.Type = "pedit"
			case strings.Contains(line, "csum"):
				currentAction.Type = "csum"
			default:
				currentAction.Type = "other"
			}
			continue
		}

		// Match conditions (indented lines before any action)
		if currentAction == nil && strings.HasPrefix(line, "  ") && !strings.Contains(line, "action") && !inActionStats {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, " ") {
				parts := strings.Fields(trimmed)
				if len(parts) >= 2 {
					currentFilter.Matches[parts[0]] = strings.Join(parts[1:], " ")
				}
			} else if trimmed != "" && trimmed != "not_in_hw" {
				currentFilter.Matches[trimmed] = "true"
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
			trimmed := strings.TrimSpace(line)
			if matches := reSentStats.FindStringSubmatch(trimmed); len(matches) == 3 {
				if bytesVal, errConv := strconv.ParseInt(matches[1], 10, 64); errConv == nil {
					currentAction.Bytes = bytesVal
				}
				if pktsVal, errConv := strconv.ParseInt(matches[2], 10, 64); errConv == nil {
					currentAction.Packets = pktsVal
				}
			}
			if matches := reDropped.FindStringSubmatch(trimmed); len(matches) == 2 {
				if dropped, errConv := strconv.ParseInt(matches[1], 10, 64); errConv == nil {
					currentAction.Dropped = dropped
				}
			}
			if matches := reOverlimits.FindStringSubmatch(trimmed); len(matches) == 2 {
				if overlimits, errConv := strconv.ParseInt(matches[1], 10, 64); errConv == nil {
					currentAction.Overlimits = overlimits
				}
			}
			if matches := reRequeues.FindStringSubmatch(trimmed); len(matches) == 2 {
				if requeues, errConv := strconv.ParseInt(matches[1], 10, 64); errConv == nil {
					currentAction.Requeues = requeues
				}
			}
			continue
		}

		// Parse backlog: "backlog 0b 0p requeues 0"
		if inActionStats && currentAction != nil && strings.Contains(line, "backlog") {
			trimmed := strings.TrimSpace(line)
			if matches := reBacklog.FindStringSubmatch(trimmed); len(matches) == 3 {
				if bytesVal, errConv := strconv.ParseInt(matches[1], 10, 64); errConv == nil {
					currentAction.BacklogBytes = bytesVal
				}
				if pktsVal, errConv := strconv.ParseInt(matches[2], 10, 64); errConv == nil {
					currentAction.BacklogPkts = pktsVal
				}
			}
			inActionStats = false
			continue
		}
	}

	if currentFilter != nil {
		if currentAction != nil {
			currentFilter.Actions = append(currentFilter.Actions, *currentAction)
		}
		filters = append(filters, *currentFilter)
	}

	return filters, nil
}

// FindMirredAction returns the first mirred action targeting dstIntf (or any mirred action if dstIntf is empty).
func (f *FilterStats) FindMirredAction(dstIntf string) *ActionStats {
	for i := range f.Actions {
		a := &f.Actions[i]
		if a.Type == "mirred" && (dstIntf == "" || a.TargetDev == dstIntf) {
			return a
		}
	}
	return nil
}

// FormatBytes converts bytes to a human-readable string.
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

// FormatByteRate formats bytes-per-second into a human-readable rate string.
func FormatByteRate(bytesPerSec float64) string {
	const unit = 1024.0
	if bytesPerSec < unit {
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
	div, exp := unit, 0
	for n := bytesPerSec / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB/s", bytesPerSec/div, "KMGTPE"[exp])
}

// GetMatchDescription returns a human-readable description of the filter match.
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
