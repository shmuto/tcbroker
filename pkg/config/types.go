package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
	"tcbroker/pkg/filter"
)

// PortConfig provides human-readable metadata for a network interface in mini-NPB mode.
type PortConfig struct {
	Label       string `yaml:"label,omitempty" json:"label,omitempty"`
	Role        string `yaml:"role,omitempty" json:"role,omitempty"` // optional: "tap" or "tool"
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// Config is the top-level declarative configuration structure.
type Config struct {
	Ports map[string]PortConfig `yaml:"ports,omitempty" json:"ports,omitempty"`
	Rules []Rule                `yaml:"rules" json:"rules"`
}

// Rule represents a traffic mirroring rule from a source interface to one or more destinations.
type Rule struct {
	Name      string                   `yaml:"name"`                 // Unique rule identifier (required)
	Src       string                   `yaml:"src"`                  // Source interface name (required)
	Direction string                   `yaml:"direction,omitempty"`  // ingress (default), egress, or both
	Dst       Destinations             `yaml:"dst"`                  // One or more destination interfaces (required)
	Rewrite   *filter.RewriteOptions   `yaml:"rewrite,omitempty"`    // Optional default rewrite for all destinations
	VLAN      *filter.VLANOptions      `yaml:"vlan,omitempty"`       // Optional default VLAN push/pop for all destinations
	RateLimit *filter.RateLimitOptions `yaml:"rate_limit,omitempty"` // Optional default mirror bandwidth cap
	Filters   []filter.Filter          `yaml:"filters,omitempty"`    // Optional match conditions (empty = all IPv4)
}

// Destination represents a mirror target interface with optional per-destination packet actions.
type Destination struct {
	Intf      string                   `yaml:"intf"`
	Rewrite   *filter.RewriteOptions   `yaml:"rewrite,omitempty"`
	VLAN      *filter.VLANOptions      `yaml:"vlan,omitempty"`
	RateLimit *filter.RateLimitOptions `yaml:"rate_limit,omitempty"`
}

// Destinations supports flexible YAML syntax:
//   - single string:   dst: eth1
//   - string list:     dst: [eth1, eth2]
//   - object or list:  dst: [{intf: eth1, vlan: {push: 100}, rewrite: {dst_mac: ...}}, eth2]
type Destinations []Destination

// UnmarshalYAML decodes flexible destination formats into a slice of Destination.
func (d *Destinations) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var intf string
		if err := value.Decode(&intf); err != nil {
			return fmt.Errorf("invalid dst value: %w", err)
		}
		*d = Destinations{{Intf: intf}}
		return nil

	case yaml.MappingNode:
		dst, err := decodeDestinationMapping(value)
		if err != nil {
			return err
		}
		*d = Destinations{dst}
		return nil

	case yaml.SequenceNode:
		result := make(Destinations, 0, len(value.Content))
		for i, item := range value.Content {
			switch item.Kind {
			case yaml.ScalarNode:
				var intf string
				if err := item.Decode(&intf); err != nil {
					return fmt.Errorf("invalid dst[%d]: %w", i, err)
				}
				result = append(result, Destination{Intf: intf})
			case yaml.MappingNode:
				dst, err := decodeDestinationMapping(item)
				if err != nil {
					return fmt.Errorf("invalid dst[%d]: %w", i, err)
				}
				result = append(result, dst)
			default:
				return fmt.Errorf("invalid dst[%d]: expected interface name or object", i)
			}
		}
		*d = result
		return nil

	default:
		return fmt.Errorf("dst must be an interface name, object, or list")
	}
}

func decodeDestinationMapping(node *yaml.Node) (Destination, error) {
	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i].Value
		switch key {
		case "intf", "rewrite", "vlan", "rate_limit":
		default:
			return Destination{}, fmt.Errorf("unknown field %q in dst object (allowed: intf, rewrite, vlan, rate_limit)", key)
		}
	}
	var dst Destination
	if err := node.Decode(&dst); err != nil {
		return Destination{}, err
	}
	return dst, nil
}

// EffectiveDirection returns the rule's direction, defaulting to "ingress".
func (r Rule) EffectiveDirection() string {
	if r.Direction == "" {
		return "ingress"
	}
	return r.Direction
}

// EffectiveHooks expands the rule's direction into TC clsact hook names.
func (r Rule) EffectiveHooks() []string {
	switch r.EffectiveDirection() {
	case "egress":
		return []string{"egress"}
	case "both":
		return []string{"ingress", "egress"}
	default:
		return []string{"ingress"}
	}
}

// EffectiveFilters returns the rule's filters, or a single match-all IPv4 filter if omitted.
func (r Rule) EffectiveFilters() []filter.Filter {
	if len(r.Filters) == 0 {
		return []filter.Filter{{}}
	}
	return r.Filters
}

// EffectiveRewrite returns the destination-specific rewrite if set, otherwise the rule-level rewrite.
func (r Rule) EffectiveRewrite(dst Destination) *filter.RewriteOptions {
	if dst.Rewrite != nil {
		return dst.Rewrite
	}
	return r.Rewrite
}

// EffectiveVLAN returns the destination-specific VLAN option if set, otherwise the rule-level VLAN option.
func (r Rule) EffectiveVLAN(dst Destination) *filter.VLANOptions {
	if dst.VLAN != nil {
		return dst.VLAN
	}
	return r.VLAN
}

// EffectiveRateLimit returns the destination-specific rate limit if set, otherwise the rule-level rate limit.
func (r Rule) EffectiveRateLimit(dst Destination) *filter.RateLimitOptions {
	if dst.RateLimit != nil {
		return dst.RateLimit
	}
	return r.RateLimit
}

// AllInterfaces returns all unique source and destination interface names referenced in the config.
func (c *Config) AllInterfaces() []string {
	seen := make(map[string]bool)
	var result []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	for _, rule := range c.Rules {
		add(rule.Src)
		for _, dst := range rule.Dst {
			add(dst.Intf)
		}
	}
	return result
}
