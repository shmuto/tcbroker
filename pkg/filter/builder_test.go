package filter

import (
	"reflect"
	"testing"
)

func TestBuildTCArgs(t *testing.T) {
	tests := []struct {
		name string
		spec BuildSpec
		want []string
	}{
		{
			name: "match all ipv4 with pref and handle",
			spec: BuildSpec{
				SrcIntf: "eth0",
				Hook:    "ingress",
				Pref:    20000,
				Handle:  "0x1",
				DstIntf: "eth1",
				Filter:  Filter{},
			},
			want: []string{
				"filter", "add", "dev", "eth0", "ingress",
				"pref", "20000", "protocol", "ip", "flower", "handle", "0x1",
				"action", "mirred", "egress", "mirror", "dev", "eth1", "continue",
			},
		},
		{
			name: "5-tuple filter without rewrite",
			spec: BuildSpec{
				SrcIntf: "eth0",
				Hook:    "egress",
				Pref:    20001,
				Handle:  "0x1",
				DstIntf: "eth2",
				Filter: Filter{
					Proto:   "tcp",
					SrcIP:   "192.168.1.0/24",
					DstIP:   "10.0.0.10",
					SrcPort: 12345,
					DstPort: 443,
				},
			},
			want: []string{
				"filter", "add", "dev", "eth0", "egress",
				"pref", "20001", "protocol", "ip", "flower", "handle", "0x1",
				"src_ip", "192.168.1.0/24",
				"dst_ip", "10.0.0.10",
				"ip_proto", "tcp",
				"src_port", "12345",
				"dst_port", "443",
				"action", "mirred", "egress", "mirror", "dev", "eth2", "continue",
			},
		},
		{
			name: "mac rewrite both dmac and smac",
			spec: BuildSpec{
				SrcIntf: "eth0",
				Hook:    "ingress",
				Pref:    20002,
				Handle:  "0x1",
				DstIntf: "eth1",
				Filter:  Filter{Proto: "udp", DstPort: 53},
				Rewrite: &RewriteOptions{
					DstMAC: "52:54:00:12:34:56",
					SrcMAC: "52:54:00:00:00:01",
				},
			},
			want: []string{
				"filter", "add", "dev", "eth0", "ingress",
				"pref", "20002", "protocol", "ip", "flower", "handle", "0x1",
				"ip_proto", "udp",
				"dst_port", "53",
				"action", "skbmod", "set", "dmac", "52:54:00:12:34:56", "smac", "52:54:00:00:00:01", "pipe",
				"action", "mirred", "egress", "mirror", "dev", "eth1", "continue",
			},
		},
		{
			name: "ip and mac rewrite with tcp csum",
			spec: BuildSpec{
				SrcIntf: "eth0",
				Hook:    "ingress",
				Pref:    20003,
				Handle:  "0x1",
				DstIntf: "eth1",
				Filter:  Filter{Proto: "tcp", DstPort: 80},
				Rewrite: &RewriteOptions{
					DstMAC: "52:54:00:12:34:56",
					DstIP:  "10.0.0.100",
					SrcIP:  "10.0.0.1",
				},
			},
			want: []string{
				"filter", "add", "dev", "eth0", "ingress",
				"pref", "20003", "protocol", "ip", "flower", "handle", "0x1",
				"ip_proto", "tcp",
				"dst_port", "80",
				"action", "skbmod", "set", "dmac", "52:54:00:12:34:56", "pipe",
				"action", "pedit", "ex", "munge", "ip", "dst", "set", "10.0.0.100", "munge", "ip", "src", "set", "10.0.0.1", "pipe",
				"action", "csum", "ip4h", "and", "tcp", "pipe",
				"action", "mirred", "egress", "mirror", "dev", "eth1", "continue",
			},
		},
		{
			name: "ip rewrite without proto recalculates all l4 csums",
			spec: BuildSpec{
				SrcIntf: "eth0",
				Hook:    "ingress",
				Pref:    20004,
				DstIntf: "eth1",
				Filter:  Filter{},
				Rewrite: &RewriteOptions{DstIP: "10.0.0.50"},
			},
			want: []string{
				"filter", "add", "dev", "eth0", "ingress",
				"pref", "20004", "protocol", "ip", "flower",
				"action", "pedit", "ex", "munge", "ip", "dst", "set", "10.0.0.50", "pipe",
				"action", "csum", "ip4h", "and", "tcp", "and", "udp", "and", "icmp", "pipe",
				"action", "mirred", "egress", "mirror", "dev", "eth1", "continue",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildTCArgs(tt.spec)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildTCArgs() mismatch\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestBuildDeleteArgs(t *testing.T) {
	got := BuildDeleteArgs("eth0", "ingress", 20001, "0x1")
	want := []string{"filter", "del", "dev", "eth0", "ingress", "pref", "20001", "handle", "0x1", "flower"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildDeleteArgs() = %q, want %q", got, want)
	}
}
