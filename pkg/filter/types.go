package filter

// Filter represents a packet filter rule.
type Filter struct {
	IPProto string `yaml:"ip_proto,omitempty"` // IP protocol (tcp, udp, icmp, etc.)
	SrcIP   string `yaml:"src_ip,omitempty"`   // Source IP address or CIDR
	DstIP   string `yaml:"dst_ip,omitempty"`   // Destination IP address or CIDR
	SrcPort int    `yaml:"src_port,omitempty"` // Source port number
	DstPort int    `yaml:"dst_port,omitempty"` // Destination port number
}
