package state

import (
	"path/filepath"
	"testing"

	"tcbroker/pkg/filter"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "state.json")

	// Loading non-existent state returns empty state
	empty, err := Load(path)
	if err != nil {
		t.Fatalf("Load() on missing file failed: %v", err)
	}
	if len(empty.Filters) != 0 || len(empty.ManagedQdiscs) != 0 {
		t.Fatalf("Expected empty state, got %+v", empty)
	}

	st := NewEmpty()
	st.ConfigPath = "/etc/tcbroker.yaml"
	st.ManagedQdiscs["eth0"] = ManagedQdisc{CreatedByTCBroker: true}
	st.Filters = append(st.Filters, ManagedFilter{
		RuleName: "web",
		SrcIntf:  "eth0",
		Hook:     "ingress",
		DstIntf:  "eth1",
		Pref:     20000,
		Handle:   "0x1",
		Filter:   filter.Filter{Proto: "tcp", DstPort: 80},
		SpecHash: ComputeSpecHash("eth0", "ingress", "eth1", filter.Filter{Proto: "tcp", DstPort: 80}, nil),
	})

	if err := Save(path, st); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.ConfigPath != st.ConfigPath {
		t.Errorf("ConfigPath = %q, want %q", loaded.ConfigPath, st.ConfigPath)
	}
	if !loaded.ManagedQdiscs["eth0"].CreatedByTCBroker {
		t.Errorf("Expected eth0 ManagedQdisc.CreatedByTCBroker=true")
	}
	if len(loaded.Filters) != 1 || loaded.Filters[0].Pref != 20000 || loaded.Filters[0].RuleName != "web" {
		t.Errorf("Unexpected loaded filters: %+v", loaded.Filters)
	}

	if err := Remove(path); err != nil {
		t.Fatalf("Remove() failed: %v", err)
	}
	// Removing again should be idempotent
	if err := Remove(path); err != nil {
		t.Fatalf("Second Remove() failed: %v", err)
	}
}

func TestComputeSpecHash_Deterministic(t *testing.T) {
	f := filter.Filter{Proto: "udp", DstPort: 53}
	h1 := ComputeSpecHash("eth0", "ingress", "eth1", f, nil)
	h2 := ComputeSpecHash("eth0", "ingress", "eth1", f, &filter.RewriteOptions{})
	if h1 != h2 {
		t.Errorf("Empty rewrite should hash identically to nil rewrite: %s != %s", h1, h2)
	}
	h3 := ComputeSpecHash("eth0", "ingress", "eth1", f, &filter.RewriteOptions{DstMAC: "aa:bb:cc:dd:ee:ff"})
	if h1 == h3 {
		t.Errorf("Non-empty rewrite should produce a different hash")
	}
}
