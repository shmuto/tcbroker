package tc

import (
	"testing"
)

func TestParseFilterStats(t *testing.T) {
	// Sample output from `tc -s filter show dev veth0 ingress`
	sampleOutput := `filter protocol ip pref 49150 flower chain 0
filter protocol ip pref 49150 flower chain 0 handle 0x1
  eth_type ipv4
  ip_proto udp
  dst_port 53
  not_in_hw
	action order 1: mirred (Egress Mirror to device veth1) pipe
	index 3 ref 1 bind 1 installed 19 sec used 19 sec
	Action statistics:
	Sent 0 bytes 0 pkt (dropped 0, overlimits 0 requeues 0)
	backlog 0b 0p requeues 0
filter protocol ip pref 49151 flower chain 0
filter protocol ip pref 49151 flower chain 0 handle 0x1
  eth_type ipv4
  ip_proto tcp
  dst_port 80
  not_in_hw
	action order 1: mirred (Egress Mirror to device veth1) pipe
	index 2 ref 1 bind 1 installed 25 sec used 10 sec
	Action statistics:
	Sent 1250 bytes 15 pkt (dropped 0, overlimits 0 requeues 0)
	backlog 0b 0p requeues 0
filter protocol ip pref 49152 flower chain 0
filter protocol ip pref 49152 flower chain 0 handle 0x1
  eth_type ipv4
  ip_proto icmp
  not_in_hw
	action order 1: mirred (Egress Mirror to device veth1) pipe
	index 1 ref 1 bind 1 installed 30 sec used 5 sec
	Action statistics:
	Sent 840 bytes 10 pkt (dropped 0, overlimits 0 requeues 0)
	backlog 0b 0p requeues 0`

	filters, err := ParseFilterStats(sampleOutput)
	if err != nil {
		t.Fatalf("ParseFilterStats failed: %v", err)
	}

	if len(filters) != 3 {
		t.Fatalf("Expected 3 filters, got %d", len(filters))
	}

	// Test first filter (UDP port 53)
	filter1 := filters[0]
	if filter1.Protocol != "ip" {
		t.Errorf("Expected protocol 'ip', got '%s'", filter1.Protocol)
	}
	if filter1.Priority != 49150 {
		t.Errorf("Expected priority 49150, got %d", filter1.Priority)
	}
	if filter1.Handle != "0x1" {
		t.Errorf("Expected handle '0x1', got '%s'", filter1.Handle)
	}
	if filter1.MatchType != "flower" {
		t.Errorf("Expected match type 'flower', got '%s'", filter1.MatchType)
	}

	// Check matches
	if filter1.Matches["ip_proto"] != "udp" {
		t.Errorf("Expected ip_proto 'udp', got '%s'", filter1.Matches["ip_proto"])
	}
	if filter1.Matches["dst_port"] != "53" {
		t.Errorf("Expected dst_port '53', got '%s'", filter1.Matches["dst_port"])
	}

	// Check action
	if len(filter1.Actions) != 1 {
		t.Fatalf("Expected 1 action, got %d", len(filter1.Actions))
	}
	action1 := filter1.Actions[0]
	if action1.Type != "mirred" {
		t.Errorf("Expected action type 'mirred', got '%s'", action1.Type)
	}
	if action1.TargetDev != "veth1" {
		t.Errorf("Expected target device 'veth1', got '%s'", action1.TargetDev)
	}
	if action1.Packets != 0 {
		t.Errorf("Expected 0 packets, got %d", action1.Packets)
	}
	if action1.Bytes != 0 {
		t.Errorf("Expected 0 bytes, got %d", action1.Bytes)
	}

	// Test second filter (TCP port 80)
	filter2 := filters[1]
	if filter2.Priority != 49151 {
		t.Errorf("Expected priority 49151, got %d", filter2.Priority)
	}
	if filter2.Matches["ip_proto"] != "tcp" {
		t.Errorf("Expected ip_proto 'tcp', got '%s'", filter2.Matches["ip_proto"])
	}
	if filter2.Matches["dst_port"] != "80" {
		t.Errorf("Expected dst_port '80', got '%s'", filter2.Matches["dst_port"])
	}

	action2 := filter2.Actions[0]
	if action2.Packets != 15 {
		t.Errorf("Expected 15 packets, got %d", action2.Packets)
	}
	if action2.Bytes != 1250 {
		t.Errorf("Expected 1250 bytes, got %d", action2.Bytes)
	}

	// Test third filter (ICMP)
	filter3 := filters[2]
	if filter3.Priority != 49152 {
		t.Errorf("Expected priority 49152, got %d", filter3.Priority)
	}
	if filter3.Matches["ip_proto"] != "icmp" {
		t.Errorf("Expected ip_proto 'icmp', got '%s'", filter3.Matches["ip_proto"])
	}

	action3 := filter3.Actions[0]
	if action3.Packets != 10 {
		t.Errorf("Expected 10 packets, got %d", action3.Packets)
	}
	if action3.Bytes != 840 {
		t.Errorf("Expected 840 bytes, got %d", action3.Bytes)
	}
}

func TestParseFilterStatsEmpty(t *testing.T) {
	filters, err := ParseFilterStats("")
	if err != nil {
		t.Fatalf("ParseFilterStats failed on empty input: %v", err)
	}
	if len(filters) != 0 {
		t.Errorf("Expected 0 filters for empty input, got %d", len(filters))
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		result := FormatBytes(tt.bytes)
		if result != tt.expected {
			t.Errorf("FormatBytes(%d) = %s, expected %s", tt.bytes, result, tt.expected)
		}
	}
}

func TestGetMatchDescription(t *testing.T) {
	tests := []struct {
		name     string
		matches  map[string]string
		expected string
	}{
		{
			name:     "ICMP only",
			matches:  map[string]string{"ip_proto": "icmp"},
			expected: "ICMP",
		},
		{
			name:     "TCP with destination port",
			matches:  map[string]string{"ip_proto": "tcp", "dst_port": "80"},
			expected: "TCP dport=80",
		},
		{
			name:     "UDP with source and destination",
			matches:  map[string]string{"ip_proto": "udp", "src_ip": "192.168.1.0/24", "dst_port": "53"},
			expected: "UDP src=192.168.1.0/24 dport=53",
		},
		{
			name:     "Empty matches",
			matches:  map[string]string{},
			expected: "ALL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := FilterStats{Matches: tt.matches}
			result := filter.GetMatchDescription()
			if result != tt.expected {
				t.Errorf("GetMatchDescription() = %s, expected %s", result, tt.expected)
			}
		})
	}
}
