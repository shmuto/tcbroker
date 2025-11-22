package config

import (
	"fmt"
	"net"
	"regexp"
)

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if len(c.Rules) == 0 {
		return fmt.Errorf("at least one rule is required")
	}

	for i, rule := range c.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("invalid rule #%d: %w", i+1, err)
		}
	}

	return nil
}

// Validate checks if the rule configuration is valid.
func (r *Rule) Validate() error {
	// Validate rule name
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Validate source interface
	if r.SrcIntf == "" {
		return fmt.Errorf("src_intf is required")
	}

	// Validate destination interface
	if r.DstIntf == "" {
		return fmt.Errorf("dst_intf is required")
	}

	// Validate rewrite options if specified
	if r.Rewrite != nil {
		if err := r.Rewrite.Validate(); err != nil {
			return fmt.Errorf("invalid rewrite options: %w", err)
		}
	}

	// Validate filters
	if len(r.Filters) == 0 {
		return fmt.Errorf("at least one filter is required")
	}

	return nil
}

// Validate checks if the rewrite options are valid.
func (r *RewriteOptions) Validate() error {
	if r == nil {
		return nil
	}

	// At least one rewrite option must be specified
	if r.DstMAC == "" && r.SrcMAC == "" && r.DstIP == "" && r.SrcIP == "" {
		return fmt.Errorf("at least one rewrite field (dst_mac, src_mac, dst_ip, src_ip) must be specified")
	}

	// Validate MAC addresses
	if r.DstMAC != "" {
		if !isValidMAC(r.DstMAC) {
			return fmt.Errorf("invalid dst_mac '%s': must be a valid MAC address (e.g., aa:bb:cc:dd:ee:ff)", r.DstMAC)
		}
	}
	if r.SrcMAC != "" {
		if !isValidMAC(r.SrcMAC) {
			return fmt.Errorf("invalid src_mac '%s': must be a valid MAC address", r.SrcMAC)
		}
	}

	// Validate IP addresses
	if r.DstIP != "" {
		if net.ParseIP(r.DstIP) == nil {
			return fmt.Errorf("invalid dst_ip '%s': must be a valid IP address", r.DstIP)
		}
	}
	if r.SrcIP != "" {
		if net.ParseIP(r.SrcIP) == nil {
			return fmt.Errorf("invalid src_ip '%s': must be a valid IP address", r.SrcIP)
		}
	}

	return nil
}

// isValidMAC checks if a string is a valid MAC address
func isValidMAC(mac string) bool {
	re := regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)
	return re.MatchString(mac)
}
