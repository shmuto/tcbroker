package filter

import (
	"fmt"
	"strings"
)

// Filter represents a 5-tuple IPv4 packet filter condition.
// An empty Filter matches all IPv4 packets.
type Filter struct {
	Proto   string `yaml:"proto,omitempty" json:"proto,omitempty"`       // IP protocol: tcp, udp, icmp (empty = any)
	SrcIP   string `yaml:"src_ip,omitempty" json:"src_ip,omitempty"`     // Source IPv4 address or CIDR
	DstIP   string `yaml:"dst_ip,omitempty" json:"dst_ip,omitempty"`     // Destination IPv4 address or CIDR
	SrcPort int    `yaml:"src_port,omitempty" json:"src_port,omitempty"` // Source L4 port (1-65535)
	DstPort int    `yaml:"dst_port,omitempty" json:"dst_port,omitempty"` // Destination L4 port (1-65535)
}

// RewriteOptions specifies packet header rewrite parameters applied before mirroring.
type RewriteOptions struct {
	DstMAC string `yaml:"dst_mac,omitempty" json:"dst_mac,omitempty"` // Destination MAC address
	SrcMAC string `yaml:"src_mac,omitempty" json:"src_mac,omitempty"` // Source MAC address
	DstIP  string `yaml:"dst_ip,omitempty" json:"dst_ip,omitempty"`   // Destination IPv4 address
	SrcIP  string `yaml:"src_ip,omitempty" json:"src_ip,omitempty"`   // Source IPv4 address
}

// IsEmpty returns true if no rewrite fields are configured.
func (r *RewriteOptions) IsEmpty() bool {
	if r == nil {
		return true
	}
	return r.DstMAC == "" && r.SrcMAC == "" && r.DstIP == "" && r.SrcIP == ""
}

// Summary returns a concise human-readable description of the filter match criteria.
func (f Filter) Summary() string {
	var parts []string
	if f.Proto != "" {
		parts = append(parts, strings.ToUpper(f.Proto))
	}
	if f.SrcIP != "" {
		parts = append(parts, fmt.Sprintf("src=%s", f.SrcIP))
	}
	if f.DstIP != "" {
		parts = append(parts, fmt.Sprintf("dst=%s", f.DstIP))
	}
	if f.SrcPort != 0 {
		parts = append(parts, fmt.Sprintf("sport=%d", f.SrcPort))
	}
	if f.DstPort != 0 {
		parts = append(parts, fmt.Sprintf("dport=%d", f.DstPort))
	}
	if len(parts) == 0 {
		return "ALL IPv4"
	}
	return strings.Join(parts, " ")
}

// Summary returns a concise human-readable description of the rewrite options.
func (r *RewriteOptions) Summary() string {
	if r.IsEmpty() {
		return ""
	}
	var parts []string
	if r.DstMAC != "" {
		parts = append(parts, "dmac="+r.DstMAC)
	}
	if r.SrcMAC != "" {
		parts = append(parts, "smac="+r.SrcMAC)
	}
	if r.DstIP != "" {
		parts = append(parts, "dip="+r.DstIP)
	}
	if r.SrcIP != "" {
		parts = append(parts, "sip="+r.SrcIP)
	}
	return strings.Join(parts, ",")
}
