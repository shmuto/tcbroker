package manager

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"tcbroker/pkg/config"
	"tcbroker/pkg/filter"
	"tcbroker/pkg/tc"
)

// fakeTC simulates kernel Linux TC state in memory to verify idempotency,
// rollback, external-tool coexistence, and drift detection without root privileges.
type fakeTC struct {
	qdiscs        map[string]bool
	filters       map[string]map[int]fakeFilter // key: "iface:hook" -> pref -> filter
	failOnAddPref int                           // if > 0, fail filter add at this pref
	addCalls      int
	delCalls      int
}

type fakeFilter struct {
	pref    int
	handle  string
	dstIntf string
	packets int64
	bytes   int64
	dropped int64
}

func newFakeTC() *fakeTC {
	return &fakeTC{
		qdiscs:  make(map[string]bool),
		filters: make(map[string]map[int]fakeFilter),
	}
}

func (f *fakeTC) Execute(args ...string) (string, string, error) {
	if len(args) == 0 {
		return "", "no args", errors.New("no args")
	}

	// Strip optional -s flag
	idx := 0
	if args[0] == "-s" {
		idx = 1
	}

	switch args[idx] {
	case "qdisc":
		return f.handleQdisc(args[idx+1:])
	case "filter":
		return f.handleFilter(args[idx+1:])
	default:
		return "", "unsupported", fmt.Errorf("unsupported command: %v", args)
	}
}

func (f *fakeTC) handleQdisc(args []string) (string, string, error) {
	if len(args) < 1 {
		return "", "", errors.New("bad qdisc args")
	}
	switch args[0] {
	case "add":
		// qdisc add dev <iface> clsact
		iface := args[2]
		if f.qdiscs[iface] {
			return "", "Error: Exclusivity flag on, cannot modify. RTNETLINK answers: File exists", errors.New("exit status 2")
		}
		f.qdiscs[iface] = true
		return "", "", nil

	case "del":
		iface := args[2]
		if !f.qdiscs[iface] {
			return "", "RTNETLINK answers: No such file or directory", errors.New("exit status 2")
		}
		delete(f.qdiscs, iface)
		delete(f.filters, iface+":ingress")
		delete(f.filters, iface+":egress")
		return "", "", nil

	case "show":
		if len(args) >= 3 && args[1] == "dev" {
			iface := args[2]
			if f.qdiscs[iface] {
				return fmt.Sprintf("qdisc clsact ffff: dev %s parent ffff:fff1\n", iface), "", nil
			}
			return "", "", nil
		}
		var sb strings.Builder
		for iface := range f.qdiscs {
			fmt.Fprintf(&sb, "qdisc clsact ffff: dev %s parent ffff:fff1\n", iface)
		}
		return sb.String(), "", nil
	}
	return "", "unknown qdisc op", errors.New("unknown qdisc op")
}

func (f *fakeTC) handleFilter(args []string) (string, string, error) {
	op := args[0]
	iface := args[2]
	hook := args[3]
	key := iface + ":" + hook

	switch op {
	case "add":
		if !f.qdiscs[iface] {
			return "", "RTNETLINK answers: No such file or directory", errors.New("exit status 2")
		}
		var pref int
		handle := "0x1"
		dstIntf := ""
		for i := 4; i < len(args); i++ {
			switch args[i] {
			case "pref":
				pref, _ = strconv.Atoi(args[i+1])
				i++
			case "handle":
				handle = args[i+1]
				i++
			case "dev":
				dstIntf = args[i+1]
				i++
			}
		}
		if f.failOnAddPref > 0 && pref == f.failOnAddPref {
			return "", "Error: simulated kernel failure", errors.New("exit status 2")
		}
		if f.filters[key] == nil {
			f.filters[key] = make(map[int]fakeFilter)
		}
		if _, exists := f.filters[key][pref]; exists {
			return "", "RTNETLINK answers: File exists", errors.New("exit status 2")
		}
		f.filters[key][pref] = fakeFilter{
			pref:    pref,
			handle:  handle,
			dstIntf: dstIntf,
		}
		f.addCalls++
		return "", "", nil

	case "del":
		var pref int
		for i := 4; i < len(args); i++ {
			if args[i] == "pref" {
				pref, _ = strconv.Atoi(args[i+1])
				i++
			}
		}
		if f.filters[key] == nil {
			return "", "RTNETLINK answers: No such file or directory", errors.New("exit status 2")
		}
		if _, exists := f.filters[key][pref]; !exists {
			return "", "RTNETLINK answers: No such file or directory", errors.New("exit status 2")
		}
		delete(f.filters[key], pref)
		f.delCalls++
		return "", "", nil

	case "show":
		if !f.qdiscs[iface] {
			return "", "", nil
		}
		var sb strings.Builder
		for _, flt := range f.filters[key] {
			fmt.Fprintf(&sb, "filter protocol ip pref %d flower chain 0\n", flt.pref)
			fmt.Fprintf(&sb, "filter protocol ip pref %d flower chain 0 handle %s\n", flt.pref, flt.handle)
			fmt.Fprintf(&sb, "  eth_type ipv4\n")
			fmt.Fprintf(&sb, "  not_in_hw\n")
			fmt.Fprintf(&sb, "\taction order 1: mirred (Egress Mirror to device %s) continue\n", flt.dstIntf)
			fmt.Fprintf(&sb, "\tindex 1 ref 1 bind 1 installed 5 sec used 1 sec\n")
			fmt.Fprintf(&sb, "\tAction statistics:\n")
			fmt.Fprintf(&sb, "\tSent %d bytes %d pkt (dropped %d, overlimits 0 requeues 0)\n", flt.bytes, flt.packets, flt.dropped)
			fmt.Fprintf(&sb, "\tbacklog 0b 0p requeues 0\n")
		}
		return sb.String(), "", nil
	}
	return "", "unknown filter op", errors.New("unknown filter op")
}

func TestManager_ApplyIdempotentAndIncremental(t *testing.T) {
	fake := newFakeTC()
	statePath := filepath.Join(t.TempDir(), "state.json")
	runner := tc.NewRunnerWithExecutor(fake, false, false, &bytes.Buffer{})
	mgr := New(runner, statePath)

	cfg1 := &config.Config{
		Rules: []config.Rule{
			{
				Name:    "http",
				Src:     "eth0",
				Dst:     config.Destinations{{Intf: "eth1"}},
				Filters: []filter.Filter{{Proto: "tcp", DstPort: 80}},
			},
		},
	}

	// First apply: creates clsact + 1 filter
	res1, err := mgr.Apply(cfg1, "cfg.yaml")
	if err != nil {
		t.Fatalf("First Apply failed: %v", err)
	}
	if len(res1.Plan.ToAdd) != 1 || len(res1.CreatedQdiscs) != 1 {
		t.Fatalf("Unexpected first apply result: add=%d createdQdiscs=%v", len(res1.Plan.ToAdd), res1.CreatedQdiscs)
	}
	pref1 := res1.Plan.ToAdd[0].Pref

	// Simulate traffic counters on first filter
	fl := fake.filters["eth0:ingress"][pref1]
	fl.packets = 42
	fl.bytes = 4200
	fake.filters["eth0:ingress"][pref1] = fl

	// Second apply with identical config: complete no-op, counters preserved!
	addsBefore := fake.addCalls
	res2, err := mgr.Apply(cfg1, "cfg.yaml")
	if err != nil {
		t.Fatalf("Second Apply failed: %v", err)
	}
	if res2.Plan.HasChanges() || fake.addCalls != addsBefore {
		t.Fatalf("Expected idempotent no-op on second apply, got add=%d del=%d", len(res2.Plan.ToAdd), len(res2.Plan.ToDelete))
	}

	// Append a second rule -> keeps first filter intact (preserving its counters!)
	cfg2 := &config.Config{
		Rules: []config.Rule{
			cfg1.Rules[0],
			{
				Name:    "https",
				Src:     "eth0",
				Dst:     config.Destinations{{Intf: "eth2"}},
				Filters: []filter.Filter{{Proto: "tcp", DstPort: 443}},
			},
		},
	}
	res3, err := mgr.Apply(cfg2, "cfg.yaml")
	if err != nil {
		t.Fatalf("Append Apply failed: %v", err)
	}
	if len(res3.Plan.Kept) != 1 || len(res3.Plan.ToAdd) != 1 || len(res3.Plan.ToDelete) != 0 {
		t.Fatalf("Expected incremental append: kept=%d add=%d del=%d", len(res3.Plan.Kept), len(res3.Plan.ToAdd), len(res3.Plan.ToDelete))
	}

	// Verify status preserves counters for 'http'
	report, err := mgr.Status("")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if len(report.Rules) != 2 || report.Rules[0].Packets != 42 || report.Rules[0].Health != HealthActive {
		t.Fatalf("Unexpected status report: %+v", report.Rules)
	}
}

func TestManager_ExternalToolsPreservedOnDelete(t *testing.T) {
	fake := newFakeTC()
	// Pre-existing clsact and external filter installed by another tool
	fake.qdiscs["eth0"] = true
	fake.filters["eth0:ingress"] = map[int]fakeFilter{
		100:   {pref: 100, handle: "0x1", dstIntf: "other0"},
		20000: {pref: 20000, handle: "0x1", dstIntf: "other1"}, // Even inside tcbroker's default range!
	}

	statePath := filepath.Join(t.TempDir(), "state.json")
	runner := tc.NewRunnerWithExecutor(fake, false, false, &bytes.Buffer{})
	mgr := New(runner, statePath)

	cfg := &config.Config{
		Rules: []config.Rule{
			{Name: "my-rule", Src: "eth0", Dst: config.Destinations{{Intf: "eth1"}}},
		},
	}

	res, err := mgr.Apply(cfg, "cfg.yaml")
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	// Should skip occupied pref 20000 and pick 20001
	if res.Plan.ToAdd[0].Pref != 20001 {
		t.Fatalf("Expected pref 20001 (skipping external 20000), got %d", res.Plan.ToAdd[0].Pref)
	}

	// Delete all tcbroker rules
	delRes, err := mgr.Delete(nil, true)
	if err != nil {
		t.Fatalf("Delete --all failed: %v", err)
	}
	if len(delRes.DeletedFilters) != 1 {
		t.Fatalf("Expected 1 deleted filter, got %d", len(delRes.DeletedFilters))
	}

	// External filters AND pre-existing clsact qdisc MUST remain untouched!
	if !fake.qdiscs["eth0"] {
		t.Fatal("Pre-existing clsact qdisc on eth0 was unexpectedly deleted!")
	}
	if _, ok := fake.filters["eth0:ingress"][100]; !ok {
		t.Fatal("External filter pref 100 was deleted!")
	}
	if _, ok := fake.filters["eth0:ingress"][20000]; !ok {
		t.Fatal("External filter pref 20000 was deleted!")
	}
	if _, ok := fake.filters["eth0:ingress"][20001]; ok {
		t.Fatal("tcbroker filter pref 20001 was not deleted!")
	}
}

func TestManager_RollbackOnFailure(t *testing.T) {
	fake := newFakeTC()
	statePath := filepath.Join(t.TempDir(), "state.json")
	runner := tc.NewRunnerWithExecutor(fake, false, false, &bytes.Buffer{})
	mgr := New(runner, statePath)

	// Apply initial valid rule
	cfg1 := &config.Config{
		Rules: []config.Rule{
			{Name: "stable", Src: "eth0", Dst: config.Destinations{{Intf: "eth1"}}},
		},
	}
	if _, err := mgr.Apply(cfg1, "cfg1.yaml"); err != nil {
		t.Fatalf("Initial Apply failed: %v", err)
	}

	// Update config with 2 modified rules; trigger failure on the 2nd filter in Gen1 (pref 25001)
	fake.failOnAddPref = Gen1Start + 1
	cfg2 := &config.Config{
		Rules: []config.Rule{
			{Name: "updated-1", Src: "eth0", Dst: config.Destinations{{Intf: "eth2"}}},
			{Name: "updated-2", Src: "eth0", Dst: config.Destinations{{Intf: "eth3"}}},
		},
	}
	if _, err := mgr.Apply(cfg2, "cfg2.yaml"); err == nil {
		t.Fatal("Expected Apply to fail on simulated kernel error")
	}

	// Original filter (pref 20000) MUST still be active and Gen1 pref 25000 must be rolled back!
	if _, ok := fake.filters["eth0:ingress"][Gen0Start]; !ok {
		t.Fatal("Original filter was lost after failed apply!")
	}
	if _, ok := fake.filters["eth0:ingress"][Gen1Start]; ok {
		t.Fatal("Partially added Gen1 filter was not rolled back!")
	}
}

func TestManager_DriftDetectionAndRecovery(t *testing.T) {
	fake := newFakeTC()
	statePath := filepath.Join(t.TempDir(), "state.json")
	runner := tc.NewRunnerWithExecutor(fake, false, false, &bytes.Buffer{})
	mgr := New(runner, statePath)

	cfg := &config.Config{
		Rules: []config.Rule{
			{Name: "dns", Src: "eth0", Dst: config.Destinations{{Intf: "eth1"}}},
		},
	}
	if _, err := mgr.Apply(cfg, "cfg.yaml"); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Simulate someone externally deleting the filter via `tc`
	delete(fake.filters["eth0:ingress"], Gen0Start)

	report, err := mgr.Status("")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if !report.HasDrift() || report.Rules[0].Health != HealthMissing {
		t.Fatalf("Expected MISSING drift, got: %+v", report.Rules[0])
	}

	// Re-applying the same config automatically heals the drift!
	res, err := mgr.Apply(cfg, "cfg.yaml")
	if err != nil {
		t.Fatalf("Re-Apply failed: %v", err)
	}
	if res.Plan.DriftedCount != 1 || len(res.Plan.ToAdd) != 1 {
		t.Fatalf("Expected drift recovery to add 1 filter, got drifted=%d add=%d", res.Plan.DriftedCount, len(res.Plan.ToAdd))
	}

	report2, err := mgr.Status("")
	if err != nil {
		t.Fatalf("Status after recovery failed: %v", err)
	}
	if report2.HasDrift() || report2.Rules[0].Health != HealthActive {
		t.Fatalf("Expected ACTIVE after recovery, got: %+v", report2.Rules[0])
	}
}

func TestManager_DeleteSingleRuleAndCleanupOwnedQdisc(t *testing.T) {
	fake := newFakeTC()
	statePath := filepath.Join(t.TempDir(), "state.json")
	runner := tc.NewRunnerWithExecutor(fake, false, false, &bytes.Buffer{})
	mgr := New(runner, statePath)

	cfg := &config.Config{
		Rules: []config.Rule{
			{Name: "r1", Src: "eth0", Dst: config.Destinations{{Intf: "eth1"}}},
			{Name: "r2", Src: "eth0", Dst: config.Destinations{{Intf: "eth2"}}},
		},
	}
	if _, err := mgr.Apply(cfg, "cfg.yaml"); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Delete only r1 -> clsact and r2 remain
	res1, err := mgr.Delete([]string{"r1"}, false)
	if err != nil {
		t.Fatalf("Delete r1 failed: %v", err)
	}
	if res1.RemainingRules != 1 || !fake.qdiscs["eth0"] {
		t.Fatalf("Expected r2 and clsact to remain: %+v", res1)
	}

	// Delete r2 -> now owned clsact should be cleanly removed
	res2, err := mgr.Delete([]string{"r2"}, false)
	if err != nil {
		t.Fatalf("Delete r2 failed: %v", err)
	}
	if res2.RemainingRules != 0 || fake.qdiscs["eth0"] || len(res2.RemovedQdiscs) != 1 {
		t.Fatalf("Expected owned clsact to be removed when empty: qdisc=%v res=%+v", fake.qdiscs["eth0"], res2)
	}
}
