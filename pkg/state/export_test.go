package state

import (
	"testing"

	"tcbroker/pkg/config"
	"tcbroker/pkg/filter"
)

func TestExportYAML_RoundTripParseable(t *testing.T) {
	st := NewEmpty()
	st.Filters = []ManagedFilter{
		{
			RuleName: "web",
			SrcIntf:  "eth0",
			Hook:     "ingress",
			DstIntf:  "eth1",
			Pref:     20000,
			Handle:   "0x1",
			Filter:   filter.Filter{Proto: "tcp", DstPort: 80},
		},
		{
			RuleName: "web",
			SrcIntf:  "eth0",
			Hook:     "ingress",
			DstIntf:  "eth2",
			Pref:     20001,
			Handle:   "0x1",
			Filter:   filter.Filter{Proto: "tcp", DstPort: 80},
			Rewrite:  &filter.RewriteOptions{DstMAC: "52:54:00:12:34:56"},
		},
		{
			RuleName: "all-both",
			SrcIntf:  "eth2",
			Hook:     "ingress",
			DstIntf:  "eth3",
			Pref:     20000,
			Handle:   "0x1",
		},
		{
			RuleName: "all-both",
			SrcIntf:  "eth2",
			Hook:     "egress",
			DstIntf:  "eth3",
			Pref:     20000,
			Handle:   "0x1",
		},
	}

	yml, err := st.ExportYAML()
	if err != nil {
		t.Fatalf("ExportYAML failed: %v", err)
	}

	parsed, err := config.Parse([]byte(yml))
	if err != nil {
		t.Fatalf("Exported YAML failed strict Parse: %v\nYAML:\n%s", err, yml)
	}
	if len(parsed.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(parsed.Rules))
	}
	if parsed.Rules[0].Name != "web" || len(parsed.Rules[0].Dst) != 2 {
		t.Errorf("unexpected first exported rule: %+v", parsed.Rules[0])
	}
	if parsed.Rules[1].Direction != "both" || len(parsed.Rules[1].Filters) != 0 {
		t.Errorf("unexpected second exported rule: %+v", parsed.Rules[1])
	}
}
