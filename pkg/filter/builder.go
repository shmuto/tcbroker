package filter

import (
	"fmt"
	"strconv"
	"strings"
)

// BuildTCArgs constructs the arguments for a `tc filter` command based on the
// provided filter criteria. This function builds arguments for use with clsact qdisc,
// where the hook (ingress/egress) itself specifies the attachment point.
func BuildTCArgs(ifaceName, hook, target string, f Filter) []string {
	args := []string{"filter", "add", "dev", ifaceName, hook}

	// Protocol is required for flower, but we can default to 'all' if not specified
	// to match any IP traffic.
	proto := "all"
	if f.IPProto != "" {
		// Flower uses 'ip_proto' to match on the protocol name.
		proto = f.IPProto
	}
	args = append(args, "protocol", "ip", "flower") // "protocol ip" is more specific than "all"

	if f.SrcIP != "" {
		args = append(args, "src_ip", f.SrcIP)
	}
	if f.DstIP != "" {
		args = append(args, "dst_ip", f.DstIP)
	}

	// Only add protocol matcher if a specific L4 protocol is given
	if f.IPProto != "" {
		args = append(args, "ip_proto", proto)
	}

	if f.SrcPort != 0 {
		args = append(args, "src_port", strconv.Itoa(f.SrcPort))
	}
	if f.DstPort != 0 {
		args = append(args, "dst_port", strconv.Itoa(f.DstPort))
	}

	args = append(args, "action", "mirred", "egress", "mirror", "dev", target, "continue")
	return args
}

// RewriteOptions specifies packet rewrite parameters.
type RewriteOptions struct {
	DstMAC string
	SrcMAC string
	DstIP  string
	SrcIP  string
}

// BuildTCArgsWithRewrite constructs tc filter arguments with packet rewrite support.
// Always uses mirror action with optional MAC/IP rewriting.
func BuildTCArgsWithRewrite(ifaceName, hook, target string, f Filter, rewrite *RewriteOptions) []string {
	args := []string{"filter", "add", "dev", ifaceName, hook}

	// Protocol setup
	proto := "all"
	if f.IPProto != "" {
		proto = f.IPProto
	}
	args = append(args, "protocol", "ip", "flower")

	// Match conditions
	if f.SrcIP != "" {
		args = append(args, "src_ip", f.SrcIP)
	}
	if f.DstIP != "" {
		args = append(args, "dst_ip", f.DstIP)
	}
	if f.IPProto != "" {
		args = append(args, "ip_proto", proto)
	}
	if f.SrcPort != 0 {
		args = append(args, "src_port", strconv.Itoa(f.SrcPort))
	}
	if f.DstPort != 0 {
		args = append(args, "dst_port", strconv.Itoa(f.DstPort))
	}

	// Add packet rewrite actions
	if rewrite != nil && (rewrite.DstMAC != "" || rewrite.SrcMAC != "" || rewrite.DstIP != "" || rewrite.SrcIP != "") {
		// MAC address rewriting using skbmod (cleaner than pedit for MAC operations)
		if rewrite.DstMAC != "" || rewrite.SrcMAC != "" {
			args = append(args, "action", "skbmod")
			if rewrite.DstMAC != "" {
				args = append(args, "set", "dmac", rewrite.DstMAC)
			}
			if rewrite.SrcMAC != "" {
				args = append(args, "set", "smac", rewrite.SrcMAC)
			}
			args = append(args, "pipe")
		}

		// IP address rewriting using pedit (skbmod doesn't support IP)
		if rewrite.DstIP != "" || rewrite.SrcIP != "" {
			args = append(args, "action", "pedit", "ex")

			if rewrite.DstIP != "" {
				args = append(args, "munge", "ip", "dst", "set", rewrite.DstIP)
			}
			if rewrite.SrcIP != "" {
				args = append(args, "munge", "ip", "src", "set", rewrite.SrcIP)
			}

			// Add checksum recalculation after IP modification
			checksums := []string{"ip"}
			if f.IPProto == "tcp" {
				checksums = append(checksums, "tcp")
			} else if f.IPProto == "udp" {
				checksums = append(checksums, "udp")
			} else if f.IPProto == "icmp" {
				checksums = append(checksums, "icmp")
			}

			args = append(args, "pipe", "action", "csum", strings.Join(checksums, " and "))
		}
	}

	// Final mirred action - always mirror
	args = append(args, "action", "mirred", "egress", "mirror", "dev", target, "continue")

	return args
}

// ValidateRewriteOptions validates rewrite options
func ValidateRewriteOptions(rewrite *RewriteOptions) error {
	if rewrite == nil {
		return nil
	}

	// Basic validation (more detailed validation should be in config package)
	if rewrite.DstMAC == "" && rewrite.SrcMAC == "" && rewrite.DstIP == "" && rewrite.SrcIP == "" {
		return fmt.Errorf("at least one rewrite option must be specified")
	}

	return nil
}
