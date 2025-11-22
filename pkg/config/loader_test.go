package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	// Create a temporary config file for testing
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `rules:
  - name: http-mirror
    src_intf: eth0
    dst_intf: eth1
    filters:
      - ip_proto: tcp
        src_ip: 192.168.1.0/24
        dst_port: 80

  - name: dns-mirror
    src_intf: eth0
    dst_intf: eth1
    filters:
      - ip_proto: udp
        dst_port: 53

  - name: https-with-rewrite
    src_intf: eth0
    dst_intf: eth1
    rewrite:
      dst_mac: "52:54:00:12:34:56"
    filters:
      - ip_proto: tcp
        dst_port: 443
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}

	// Check that we have rules
	if len(cfg.Rules) != 3 {
		t.Fatalf("Expected 3 rules, got %d", len(cfg.Rules))
	}

	// Check first rule (HTTP mirror)
	rule1 := cfg.Rules[0]
	if rule1.Name != "http-mirror" {
		t.Errorf("Expected rule1 name 'http-mirror', got '%s'", rule1.Name)
	}
	if rule1.SrcIntf != "eth0" {
		t.Errorf("Expected rule1 src_intf 'eth0', got '%s'", rule1.SrcIntf)
	}
	if rule1.DstIntf != "eth1" {
		t.Errorf("Expected rule1 dst_intf 'eth1', got '%s'", rule1.DstIntf)
	}
	if len(rule1.Filters) != 1 {
		t.Fatalf("Expected 1 filter in rule1, got %d", len(rule1.Filters))
	}

	filter1 := rule1.Filters[0]
	if filter1.IPProto != "tcp" {
		t.Errorf("Expected filter1 ip_proto 'tcp', got '%s'", filter1.IPProto)
	}
	if filter1.SrcIP != "192.168.1.0/24" {
		t.Errorf("Expected filter1 src_ip '192.168.1.0/24', got '%s'", filter1.SrcIP)
	}
	if filter1.DstPort != 80 {
		t.Errorf("Expected filter1 dst_port 80, got %d", filter1.DstPort)
	}

	// Check second rule (DNS mirror)
	rule2 := cfg.Rules[1]
	if rule2.Name != "dns-mirror" {
		t.Errorf("Expected rule2 name 'dns-mirror', got '%s'", rule2.Name)
	}

	filter2 := rule2.Filters[0]
	if filter2.IPProto != "udp" {
		t.Errorf("Expected filter2 ip_proto 'udp', got '%s'", filter2.IPProto)
	}
	if filter2.DstPort != 53 {
		t.Errorf("Expected filter2 dst_port 53, got %d", filter2.DstPort)
	}

	// Check third rule with rewrite
	rule3 := cfg.Rules[2]
	if rule3.Name != "https-with-rewrite" {
		t.Errorf("Expected rule3 name 'https-with-rewrite', got '%s'", rule3.Name)
	}

	if rule3.Rewrite == nil {
		t.Fatal("Expected rule3 to have rewrite options")
	}

	if rule3.Rewrite.DstMAC != "52:54:00:12:34:56" {
		t.Errorf("Expected rule3 rewrite dst_mac '52:54:00:12:34:56', got '%s'", rule3.Rewrite.DstMAC)
	}
}
