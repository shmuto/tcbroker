package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
	"tcbroker/pkg/filter"
)

// Load reads a YAML file from the given path, parses it strictly, normalizes defaults,
// and validates it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}
	return Parse(data)
}

// Parse decodes YAML bytes with strict unknown-field checking, normalizes defaults,
// and validates the resulting Config.
func Parse(data []byte) (*Config, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("config file is empty")
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("config file is empty")
		}
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	cfg.normalize()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

func (c *Config) normalize() {
	for i := range c.Rules {
		r := &c.Rules[i]
		r.Name = strings.TrimSpace(r.Name)
		r.Src = strings.TrimSpace(r.Src)
		r.Direction = strings.ToLower(strings.TrimSpace(r.Direction))
		if r.Direction == "" {
			r.Direction = "ingress"
		}
		for j := range r.Dst {
			r.Dst[j].Intf = strings.TrimSpace(r.Dst[j].Intf)
			normalizeRewrite(r.Dst[j].Rewrite)
		}
		normalizeRewrite(r.Rewrite)
		for j := range r.Filters {
			r.Filters[j].Proto = strings.ToLower(strings.TrimSpace(r.Filters[j].Proto))
			r.Filters[j].SrcIP = strings.TrimSpace(r.Filters[j].SrcIP)
			r.Filters[j].DstIP = strings.TrimSpace(r.Filters[j].DstIP)
		}
	}
}

func normalizeRewrite(rw *filter.RewriteOptions) {
	if rw == nil {
		return
	}
	rw.DstMAC = strings.ToLower(strings.TrimSpace(rw.DstMAC))
	rw.SrcMAC = strings.ToLower(strings.TrimSpace(rw.SrcMAC))
	rw.DstIP = strings.TrimSpace(rw.DstIP)
	rw.SrcIP = strings.TrimSpace(rw.SrcIP)
}
