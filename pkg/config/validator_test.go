package config

import (
	"testing"

	"tcbroker/pkg/filter"
)

func TestConfig_Validate(t *testing.T) {
	testCases := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid minimal rule without filters",
			config: &Config{
				Rules: []Rule{
					{
						Name: "all-ipv4",
						Src:  "eth0",
						Dst:  Destinations{{Intf: "eth1"}},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid multi-destination with rewrite and direction",
			config: &Config{
				Rules: []Rule{
					{
						Name:      "web-mirror",
						Src:       "eth0",
						Direction: "both",
						Dst: Destinations{
							{Intf: "eth1"},
							{Intf: "eth2", Rewrite: &filter.RewriteOptions{DstMAC: "52:54:00:12:34:56", DstIP: "10.0.0.10"}},
						},
						Filters: []filter.Filter{
							{Proto: "tcp", SrcIP: "192.168.1.0/24", DstPort: 443},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "no rules",
			config:  &Config{},
			wantErr: true,
		},
		{
			name: "duplicate rule names",
			config: &Config{
				Rules: []Rule{
					{Name: "dup", Src: "eth0", Dst: Destinations{{Intf: "eth1"}}},
					{Name: "dup", Src: "eth0", Dst: Destinations{{Intf: "eth2"}}},
				},
			},
			wantErr: true,
		},
		{
			name: "missing name",
			config: &Config{
				Rules: []Rule{
					{Src: "eth0", Dst: Destinations{{Intf: "eth1"}}},
				},
			},
			wantErr: true,
		},
		{
			name: "missing src",
			config: &Config{
				Rules: []Rule{
					{Name: "r1", Dst: Destinations{{Intf: "eth1"}}},
				},
			},
			wantErr: true,
		},
		{
			name: "missing dst",
			config: &Config{
				Rules: []Rule{
					{Name: "r1", Src: "eth0"},
				},
			},
			wantErr: true,
		},
		{
			name: "self mirror loop",
			config: &Config{
				Rules: []Rule{
					{Name: "loop", Src: "eth0", Dst: Destinations{{Intf: "eth0"}}},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid direction",
			config: &Config{
				Rules: []Rule{
					{Name: "r1", Src: "eth0", Direction: "inbound", Dst: Destinations{{Intf: "eth1"}}},
				},
			},
			wantErr: true,
		},
		{
			name: "port without tcp/udp proto",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "r1",
						Src:     "eth0",
						Dst:     Destinations{{Intf: "eth1"}},
						Filters: []filter.Filter{{DstPort: 80}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "port with icmp proto",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "r1",
						Src:     "eth0",
						Dst:     Destinations{{Intf: "eth1"}},
						Filters: []filter.Filter{{Proto: "icmp", DstPort: 80}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "port out of range",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "r1",
						Src:     "eth0",
						Dst:     Destinations{{Intf: "eth1"}},
						Filters: []filter.Filter{{Proto: "tcp", DstPort: 70000}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid proto",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "r1",
						Src:     "eth0",
						Dst:     Destinations{{Intf: "eth1"}},
						Filters: []filter.Filter{{Proto: "sctp"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid src_ip CIDR",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "r1",
						Src:     "eth0",
						Dst:     Destinations{{Intf: "eth1"}},
						Filters: []filter.Filter{{SrcIP: "999.1.1.1/24"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "ipv6 not supported",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "r1",
						Src:     "eth0",
						Dst:     Destinations{{Intf: "eth1"}},
						Filters: []filter.Filter{{SrcIP: "2001:db8::1"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid MAC address",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "r1",
						Src:     "eth0",
						Dst:     Destinations{{Intf: "eth1"}},
						Rewrite: &filter.RewriteOptions{DstMAC: "invalid-mac"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid rewrite IP",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "r1",
						Src:     "eth0",
						Dst:     Destinations{{Intf: "eth1"}},
						Rewrite: &filter.RewriteOptions{DstIP: "not-an-ip"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty rewrite struct",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "r1",
						Src:     "eth0",
						Dst:     Destinations{{Intf: "eth1"}},
						Rewrite: &filter.RewriteOptions{},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestConfig_Warnings(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name:    "rw-rule",
				Src:     "eth0",
				Dst:     Destinations{{Intf: "eth1"}},
				Rewrite: &filter.RewriteOptions{DstMAC: "52:54:00:12:34:56"},
			},
			{
				Name: "plain-rule",
				Src:  "eth0",
				Dst:  Destinations{{Intf: "eth2"}},
			},
		},
	}

	warnings := cfg.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("Expected 1 warning for shared interface with rewrite, got %d: %v", len(warnings), warnings)
	}
}
