package config

import "tcbroker/pkg/filter"

// Config is the top-level configuration structure.
type Config struct {
	Rules []Rule `yaml:"rules"`
}

// Rule represents a traffic mirroring rule.
type Rule struct {
	Name    string          `yaml:"name"`              // Rule name for identification (required)
	SrcIntf string          `yaml:"src_intf"`          // Source interface name
	DstIntf string          `yaml:"dst_intf"`          // Destination interface name
	Rewrite *RewriteOptions `yaml:"rewrite,omitempty"` // Optional packet rewrite options
	Filters []filter.Filter `yaml:"filters"`           // Filter conditions
}

// RewriteOptions specifies packet rewrite parameters for redirect mode.
type RewriteOptions struct {
	DstMAC string `yaml:"dst_mac,omitempty"` // Destination MAC address
	SrcMAC string `yaml:"src_mac,omitempty"` // Source MAC address
	DstIP  string `yaml:"dst_ip,omitempty"`  // Destination IP address
	SrcIP  string `yaml:"src_ip,omitempty"`  // Source IP address (for SNAT)
}
