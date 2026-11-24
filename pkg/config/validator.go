package config

import (
	"fmt"
	"net"
	"strings"

	"tcbroker/pkg/filter"
)

// Validate checks if the entire configuration is valid.
func (c *Config) Validate() error {
	if len(c.Rules) == 0 {
		return fmt.Errorf("at least one rule is required under 'rules'")
	}

	seenNames := make(map[string]int)
	for i, rule := range c.Rules {
		if err := rule.Validate(); err != nil {
			label := fmt.Sprintf("rule #%d", i+1)
			if rule.Name != "" {
				label = fmt.Sprintf("rule %q (#%d)", rule.Name, i+1)
			}
			return fmt.Errorf("invalid %s: %w", label, err)
		}
		if prevIdx, exists := seenNames[rule.Name]; exists {
			return fmt.Errorf("duplicate rule name %q (used in rule #%d and rule #%d)", rule.Name, prevIdx, i+1)
		}
		seenNames[rule.Name] = i + 1
	}

	return nil
}

// Validate checks if a single rule configuration is valid.
func (r *Rule) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}

	if strings.TrimSpace(r.Src) == "" {
		return fmt.Errorf("src is required")
	}

	dir := r.EffectiveDirection()
	switch dir {
	case "ingress", "egress", "both":
	default:
		return fmt.Errorf("invalid direction %q: must be one of ingress, egress, both", r.Direction)
	}

	if len(r.Dst) == 0 {
		return fmt.Errorf("at least one destination (dst) is required")
	}

	if r.Rewrite != nil {
		if err := validateRewrite(r.Rewrite); err != nil {
			return fmt.Errorf("invalid rewrite: %w", err)
		}
	}

	for i, dst := range r.Dst {
		if strings.TrimSpace(dst.Intf) == "" {
			return fmt.Errorf("dst[%d].intf is required", i)
		}
		if dst.Intf == r.Src {
			return fmt.Errorf("dst[%d].intf %q cannot be the same as src (would cause a packet loop)", i, dst.Intf)
		}
		if dst.Rewrite != nil {
			if err := validateRewrite(dst.Rewrite); err != nil {
				return fmt.Errorf("invalid dst[%d].rewrite: %w", i, err)
			}
		}
	}

	for i, f := range r.Filters {
		if err := validateFilter(f); err != nil {
			return fmt.Errorf("invalid filters[%d]: %w", i, err)
		}
	}

	return nil
}

func validateFilter(f filter.Filter) error {
	proto := strings.ToLower(strings.TrimSpace(f.Proto))
	if proto != "" {
		switch proto {
		case "tcp", "udp", "icmp":
		default:
			return fmt.Errorf("invalid proto %q: must be one of tcp, udp, icmp", f.Proto)
		}
	}

	if f.SrcIP != "" {
		if err := validateIPv4OrCIDR(f.SrcIP); err != nil {
			return fmt.Errorf("invalid src_ip %q: %w", f.SrcIP, err)
		}
	}
	if f.DstIP != "" {
		if err := validateIPv4OrCIDR(f.DstIP); err != nil {
			return fmt.Errorf("invalid dst_ip %q: %w", f.DstIP, err)
		}
	}

	if f.SrcPort < 0 || f.SrcPort > 65535 {
		return fmt.Errorf("invalid src_port %d: must be between 1 and 65535", f.SrcPort)
	}
	if f.DstPort < 0 || f.DstPort > 65535 {
		return fmt.Errorf("invalid dst_port %d: must be between 1 and 65535", f.DstPort)
	}

	if (f.SrcPort != 0 || f.DstPort != 0) && proto != "tcp" && proto != "udp" {
		return fmt.Errorf("src_port/dst_port requires proto to be 'tcp' or 'udp' (got %q)", f.Proto)
	}

	return nil
}

func validateRewrite(rw *filter.RewriteOptions) error {
	if rw == nil {
		return nil
	}
	if rw.IsEmpty() {
		return fmt.Errorf("at least one field (dst_mac, src_mac, dst_ip, src_ip) must be specified")
	}

	if rw.DstMAC != "" {
		if !isValidMAC(rw.DstMAC) {
			return fmt.Errorf("invalid dst_mac %q: must be a 6-byte MAC address (e.g. 52:54:00:12:34:56)", rw.DstMAC)
		}
	}
	if rw.SrcMAC != "" {
		if !isValidMAC(rw.SrcMAC) {
			return fmt.Errorf("invalid src_mac %q: must be a 6-byte MAC address (e.g. 52:54:00:12:34:56)", rw.SrcMAC)
		}
	}
	if rw.DstIP != "" {
		if !isValidIPv4(rw.DstIP) {
			return fmt.Errorf("invalid dst_ip %q: must be a valid IPv4 address", rw.DstIP)
		}
	}
	if rw.SrcIP != "" {
		if !isValidIPv4(rw.SrcIP) {
			return fmt.Errorf("invalid src_ip %q: must be a valid IPv4 address", rw.SrcIP)
		}
	}

	return nil
}

func isValidMAC(mac string) bool {
	hw, err := net.ParseMAC(mac)
	return err == nil && len(hw) == 6
}

func isValidIPv4(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	return ip != nil && ip.To4() != nil
}

func validateIPv4OrCIDR(val string) error {
	if strings.Contains(val, "/") {
		ip, _, err := net.ParseCIDR(val)
		if err != nil || ip.To4() == nil {
			return fmt.Errorf("must be a valid IPv4 address or CIDR (e.g. 192.168.1.0/24)")
		}
		return nil
	}
	if !isValidIPv4(val) {
		return fmt.Errorf("must be a valid IPv4 address or CIDR (e.g. 192.168.1.10 or 192.168.1.0/24)")
	}
	return nil
}

// Warnings returns non-fatal operational warnings about the configuration.
func (c *Config) Warnings() []string {
	type ifaceHook struct {
		src  string
		hook string
	}
	counts := make(map[ifaceHook]int)
	rewriteRules := make(map[ifaceHook][]string)

	for _, rule := range c.Rules {
		for _, hook := range rule.EffectiveHooks() {
			key := ifaceHook{src: rule.Src, hook: hook}
			hasRewrite := false
			for _, dst := range rule.Dst {
				counts[key]++
				if !rule.EffectiveRewrite(dst).IsEmpty() {
					hasRewrite = true
				}
			}
			if hasRewrite {
				rewriteRules[key] = append(rewriteRules[key], rule.Name)
			}
		}
	}

	var warnings []string
	for key, names := range rewriteRules {
		if counts[key] > 1 {
			warnings = append(warnings, fmt.Sprintf(
				"interface %q (%s) has rewrite rule(s) [%s] alongside other mirror targets; Linux TC rewrites packets in-place before subsequent rules and normal host delivery",
				key.src, key.hook, strings.Join(names, ", "),
			))
		}
	}
	return warnings
}
