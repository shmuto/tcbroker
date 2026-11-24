package state

import (
	"reflect"

	"gopkg.in/yaml.v3"
	"tcbroker/pkg/config"
	"tcbroker/pkg/filter"
)

// ExportConfig reconstructs a declarative config.Config from the currently persisted filters.
// This enables editing, exporting YAML, and reconciling drift even without the original file.
func (s *State) ExportConfig() *config.Config {
	cfg := &config.Config{Rules: []config.Rule{}}
	if s == nil || len(s.Filters) == 0 {
		return cfg
	}

	type ruleBuilder struct {
		name      string
		src       string
		hasIn     bool
		hasOut    bool
		dsts      []config.Destination
		filters   []filter.Filter
		allFilter bool
	}

	order := []string{}
	builders := make(map[string]*ruleBuilder)

	for _, mf := range s.Filters {
		rb, exists := builders[mf.RuleName]
		if !exists {
			rb = &ruleBuilder{
				name: mf.RuleName,
				src:  mf.SrcIntf,
			}
			builders[mf.RuleName] = rb
			order = append(order, mf.RuleName)
		}

		switch mf.Hook {
		case "ingress":
			rb.hasIn = true
		case "egress":
			rb.hasOut = true
		}

		// Deduplicate destination + rewrite pair
		dst := config.Destination{Intf: mf.DstIntf}
		if !mf.Rewrite.IsEmpty() {
			rwCopy := *mf.Rewrite
			dst.Rewrite = &rwCopy
		}
		foundDst := false
		for _, existing := range rb.dsts {
			if existing.Intf == dst.Intf && reflect.DeepEqual(existing.Rewrite, dst.Rewrite) {
				foundDst = true
				break
			}
		}
		if !foundDst {
			rb.dsts = append(rb.dsts, dst)
		}

		// Deduplicate filters
		if mf.Filter == (filter.Filter{}) {
			rb.allFilter = true
		} else {
			foundFilter := false
			for _, ef := range rb.filters {
				if ef == mf.Filter {
					foundFilter = true
					break
				}
			}
			if !foundFilter {
				rb.filters = append(rb.filters, mf.Filter)
			}
		}
	}

	for _, name := range order {
		rb := builders[name]
		dir := "ingress"
		switch {
		case rb.hasIn && rb.hasOut:
			dir = "both"
		case rb.hasOut && !rb.hasIn:
			dir = "egress"
		}

		rule := config.Rule{
			Name:      rb.name,
			Src:       rb.src,
			Direction: dir,
			Dst:       rb.dsts,
		}
		if len(rb.filters) > 0 && !rb.allFilter {
			rule.Filters = rb.filters
		}
		cfg.Rules = append(cfg.Rules, rule)
	}

	return cfg
}

// ExportYAML renders the reconstructed Config as formatted YAML.
func (s *State) ExportYAML() (string, error) {
	cfg := s.ExportConfig()
	if len(cfg.Rules) == 0 {
		return "rules: []\n", nil
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
