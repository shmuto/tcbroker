package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tcbroker.yaml")

	configContent := `rules:
  - name: mirror-all
    src: eth0
    dst: eth1

  - name: web-multi
    src: eth0
    direction: both
    dst:
      - eth1
      - intf: eth2
        rewrite:
          dst_mac: "52:54:00:12:34:56"
    filters:
      - proto: TCP
        src_ip: 192.168.1.0/24
        dst_port: 80

  - name: rule-rewrite
    src: eth1
    dst:
      intf: eth2
    rewrite:
      dst_ip: "10.0.0.100"
    filters:
      - proto: udp
        dst_port: 53
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(cfg.Rules) != 3 {
		t.Fatalf("Expected 3 rules, got %d", len(cfg.Rules))
	}

	// Rule 1: minimal (filters omitted, direction defaults to ingress)
	r1 := cfg.Rules[0]
	if r1.Name != "mirror-all" || r1.Src != "eth0" || r1.EffectiveDirection() != "ingress" {
		t.Errorf("Unexpected rule1 metadata: %+v", r1)
	}
	if len(r1.Dst) != 1 || r1.Dst[0].Intf != "eth1" {
		t.Errorf("Unexpected rule1 dst: %+v", r1.Dst)
	}
	if len(r1.EffectiveFilters()) != 1 {
		t.Errorf("Expected 1 effective catch-all filter, got %d", len(r1.EffectiveFilters()))
	}

	// Rule 2: multi-dst + mixed string/object + normalized proto + direction both
	r2 := cfg.Rules[1]
	if len(r2.EffectiveHooks()) != 2 {
		t.Errorf("Expected 2 hooks for direction=both, got %v", r2.EffectiveHooks())
	}
	if len(r2.Dst) != 2 || r2.Dst[0].Intf != "eth1" || r2.Dst[1].Intf != "eth2" {
		t.Fatalf("Unexpected rule2 dst: %+v", r2.Dst)
	}
	if r2.Dst[1].Rewrite == nil || r2.Dst[1].Rewrite.DstMAC != "52:54:00:12:34:56" {
		t.Errorf("Unexpected rule2 dst[1] rewrite: %+v", r2.Dst[1].Rewrite)
	}
	if r2.Filters[0].Proto != "tcp" {
		t.Errorf("Expected normalized proto 'tcp', got %q", r2.Filters[0].Proto)
	}

	// Rule 3: object dst + rule-level rewrite fallback
	r3 := cfg.Rules[2]
	if eff := r3.EffectiveRewrite(r3.Dst[0]); eff == nil || eff.DstIP != "10.0.0.100" {
		t.Errorf("Expected rule-level rewrite fallback, got %+v", eff)
	}
}

func TestParse_UnknownFieldRejected(t *testing.T) {
	yamlData := `rules:
  - name: typo-test
    src: eth0
    dst: eth1
    filters:
      - proto: tcp
        dst_prot: 80
`
	_, err := Parse([]byte(yamlData))
	if err == nil {
		t.Fatal("Expected error for unknown field 'dst_prot', got nil")
	}
	if !strings.Contains(err.Error(), "dst_prot") {
		t.Errorf("Expected error mentioning 'dst_prot', got: %v", err)
	}
}

func TestParse_SampleTemplateIsValid(t *testing.T) {
	cfg, err := Parse([]byte(SampleConfigTemplate))
	if err != nil {
		t.Fatalf("SampleConfigTemplate failed validation: %v", err)
	}
	if len(cfg.Rules) == 0 {
		t.Fatal("SampleConfigTemplate should define at least one rule")
	}
}
