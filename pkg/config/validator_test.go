package config

import (
	"tcbroker/pkg/filter"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	testCases := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config with mirror",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "test-rule",
						SrcIntf: "eth0",
						DstIntf: "eth1",
						Filters: []filter.Filter{
							{IPProto: "tcp"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with rewrite",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "test-rule-rewrite",
						SrcIntf: "eth0",
						DstIntf: "eth1",
						Rewrite: &RewriteOptions{
							DstMAC: "52:54:00:12:34:56",
						},
						Filters: []filter.Filter{
							{IPProto: "tcp"},
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
			name: "missing name",
			config: &Config{
				Rules: []Rule{
					{
						SrcIntf: "eth0",
						DstIntf: "eth1",
						Filters: []filter.Filter{{IPProto: "tcp"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing src_intf",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "test-rule",
						DstIntf: "eth1",
						Filters: []filter.Filter{{IPProto: "tcp"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing dst_intf",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "test-rule",
						SrcIntf: "eth0",
						Filters: []filter.Filter{{IPProto: "tcp"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no filters",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "test-rule",
						SrcIntf: "eth0",
						DstIntf: "eth1",
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
						Name:    "test-rule",
						SrcIntf: "eth0",
						DstIntf: "eth1",
						Rewrite: &RewriteOptions{
							DstMAC: "invalid-mac",
						},
						Filters: []filter.Filter{{IPProto: "tcp"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid IP address",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "test-rule",
						SrcIntf: "eth0",
						DstIntf: "eth1",
						Rewrite: &RewriteOptions{
							DstIP: "invalid-ip",
						},
						Filters: []filter.Filter{{IPProto: "tcp"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "rewrite with no options",
			config: &Config{
				Rules: []Rule{
					{
						Name:    "test-rule",
						SrcIntf: "eth0",
						DstIntf: "eth1",
						Rewrite: &RewriteOptions{},
						Filters: []filter.Filter{{IPProto: "tcp"}},
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
