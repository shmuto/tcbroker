package filter

import (
	"strconv"
)

// BuildSpec holds all parameters required to construct a managed `tc filter add` command.
type BuildSpec struct {
	SrcIntf   string
	Hook      string // "ingress" or "egress"
	Pref      int    // TC filter priority (1-65535); 0 omits explicit pref
	Handle    string // TC filter handle (e.g., "0x1"); empty omits explicit handle
	DstIntf   string
	Filter    Filter
	Rewrite   *RewriteOptions
	VLAN      *VLANOptions
	RateLimit *RateLimitOptions
}

// BuildTCArgs constructs arguments for `tc filter add` using clsact + flower + optional
// police / skbmod / pedit / csum / vlan + mirred mirror.
func BuildTCArgs(spec BuildSpec) []string {
	args := []string{"filter", "add", "dev", spec.SrcIntf, spec.Hook}

	if spec.Pref > 0 {
		args = append(args, "pref", strconv.Itoa(spec.Pref))
	}
	args = append(args, "protocol", "ip", "flower")
	if spec.Handle != "" {
		args = append(args, "handle", spec.Handle)
	}

	// Match conditions
	f := spec.Filter
	if f.SrcIP != "" {
		args = append(args, "src_ip", f.SrcIP)
	}
	if f.DstIP != "" {
		args = append(args, "dst_ip", f.DstIP)
	}
	if f.Proto != "" {
		args = append(args, "ip_proto", f.Proto)
	}
	if f.SrcPort != 0 {
		args = append(args, "src_port", strconv.Itoa(f.SrcPort))
	}
	if f.DstPort != 0 {
		args = append(args, "dst_port", strconv.Itoa(f.DstPort))
	}

	// Optional safe rate limiting: conform-exceed continue/pipe skips mirroring on exceed
	// without ever dropping original production traffic.
	if !spec.RateLimit.IsEmpty() {
		args = append(args,
			"action", "police",
			"rate", spec.RateLimit.Rate,
			"burst", spec.RateLimit.EffectiveBurst(),
			"conform-exceed", "continue/pipe",
		)
	}

	// Optional packet header rewrite actions
	if !spec.Rewrite.IsEmpty() {
		rw := spec.Rewrite

		// MAC address rewriting using skbmod
		if rw.DstMAC != "" || rw.SrcMAC != "" {
			args = append(args, "action", "skbmod", "set")
			if rw.DstMAC != "" {
				args = append(args, "dmac", rw.DstMAC)
			}
			if rw.SrcMAC != "" {
				args = append(args, "smac", rw.SrcMAC)
			}
			args = append(args, "pipe")
		}

		// IPv4 address rewriting using pedit + checksum recalculation using csum
		if rw.DstIP != "" || rw.SrcIP != "" {
			args = append(args, "action", "pedit", "ex")
			if rw.DstIP != "" {
				args = append(args, "munge", "ip", "dst", "set", rw.DstIP)
			}
			if rw.SrcIP != "" {
				args = append(args, "munge", "ip", "src", "set", rw.SrcIP)
			}
			args = append(args, "pipe")

			// Recalculate IPv4 header checksum and L4 checksum(s).
			args = append(args, "action", "csum", "ip4h")
			switch f.Proto {
			case "tcp":
				args = append(args, "and", "tcp")
			case "udp":
				args = append(args, "and", "udp")
			case "icmp":
				args = append(args, "and", "icmp")
			default:
				args = append(args, "and", "tcp", "and", "udp", "and", "icmp")
			}
			args = append(args, "pipe")
		}
	}

	// Optional VLAN tag manipulation
	if !spec.VLAN.IsEmpty() {
		if spec.VLAN.Push > 0 {
			// Push VLAN tag for the mirrored copy, then pop it back after mirred so the
			// original packet continues unmodified.
			args = append(args, "action", "vlan", "push", "id", strconv.Itoa(spec.VLAN.Push), "pipe")
			args = append(args, "action", "mirred", "egress", "mirror", "dev", spec.DstIntf, "pipe")
			args = append(args, "action", "vlan", "pop", "continue")
			return args
		}
		if spec.VLAN.Pop {
			args = append(args, "action", "vlan", "pop", "pipe")
		}
	}

	// Final mirred mirror action; continue lets subsequent filters evaluate the packet
	args = append(args, "action", "mirred", "egress", "mirror", "dev", spec.DstIntf, "continue")
	return args
}

// BuildDeleteArgs constructs arguments for deleting a single managed filter by pref.
func BuildDeleteArgs(srcIntf, hook string, pref int, handle string) []string {
	args := []string{"filter", "del", "dev", srcIntf, hook, "pref", strconv.Itoa(pref)}
	if handle != "" {
		args = append(args, "handle", handle, "flower")
	}
	return args
}
