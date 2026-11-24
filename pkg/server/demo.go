package server

import (
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"tcbroker/pkg/config"
	"tcbroker/pkg/filter"
)

// DemoExecutor simulates an in-memory Linux TC kernel with live traffic counters
// so users can explore the mini-NPB WebUI without root privileges or real NICs.
type DemoExecutor struct {
	mu      sync.Mutex
	qdiscs  map[string]bool
	filters map[string]map[int]*demoFilter
	rng     *rand.Rand
}

type demoFilter struct {
	pref       int
	handle     string
	dstIntf    string
	packets    int64
	bytes      int64
	dropped    int64
	ppsBase    int64
	avgPktSize int64
	lastTick   time.Time
}

// NewDemoExecutor creates an initialized DemoExecutor.
func NewDemoExecutor() *DemoExecutor {
	return &DemoExecutor{
		qdiscs:  make(map[string]bool),
		filters: make(map[string]map[int]*demoFilter),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// DemoInterfaces returns realistic virtual NIC metadata for demo mode.
func DemoInterfaces() []InterfaceInfo {
	return []InterfaceInfo{
		{Name: "eth0", State: "UP", MAC: "52:54:00:10:00:01"},
		{Name: "eth1", State: "UP", MAC: "52:54:00:10:00:02"},
		{Name: "eth2", State: "UP", MAC: "52:54:00:20:00:01"},
		{Name: "tap-ids0", State: "UP", MAC: "52:54:00:30:00:01"},
		{Name: "tap-zeek0", State: "UP", MAC: "52:54:00:30:00:02"},
		{Name: "tap-stg0", State: "UP", MAC: "52:54:00:40:00:01"},
	}
}

// DemoInitialConfig returns a realistic mini-NPB starter config for demo mode.
func DemoInitialConfig() *config.Config {
	return &config.Config{
		Ports: map[string]config.PortConfig{
			"eth0":      {Label: "wan-uplink", Role: "tap"},
			"eth2":      {Label: "dmz-tap", Role: "tap"},
			"tap-ids0":  {Label: "suricata-ids", Role: "tool"},
			"tap-zeek0": {Label: "zeek-ndr", Role: "tool"},
			"tap-stg0":  {Label: "staging-replay", Role: "tool"},
		},
		Rules: []config.Rule{
			{
				Name:      "wan-web-inspect",
				Src:       "eth0",
				Direction: "ingress",
				RateLimit: &filter.RateLimitOptions{Rate: "500mbit"},
				Dst: config.Destinations{
					{Intf: "tap-ids0", VLAN: &filter.VLANOptions{Push: 100}},
					{Intf: "tap-zeek0"},
				},
				Filters: []filter.Filter{
					{Proto: "tcp", DstPort: 443},
					{Proto: "tcp", DstPort: 80},
				},
			},
			{
				Name:      "dns-shadow-replay",
				Src:       "eth0",
				Direction: "ingress",
				Dst: config.Destinations{
					{
						Intf: "tap-stg0",
						Rewrite: &filter.RewriteOptions{
							DstIP:  "10.99.0.53",
							DstMAC: "52:54:00:40:00:99",
						},
					},
				},
				Filters: []filter.Filter{
					{Proto: "udp", DstPort: 53},
				},
			},
			{
				Name:      "dmz-audit-all",
				Src:       "eth2",
				Direction: "both",
				Dst: config.Destinations{
					{Intf: "tap-zeek0", VLAN: &filter.VLANOptions{Push: 200}},
				},
			},
		},
	}
}

// SimulateDrift removes one live filter from the simulated kernel without updating state.json
// so the user can test drift detection and one-click reconciliation in demo mode.
func (d *DemoExecutor) SimulateDrift() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for key, byPref := range d.filters {
		for pref := range byPref {
			delete(byPref, pref)
			if len(byPref) == 0 {
				delete(d.filters, key)
			}
			return true
		}
	}
	return false
}

// Execute implements tc.CommandExecutor against in-memory state.
func (d *DemoExecutor) Execute(args ...string) (string, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(args) == 0 {
		return "", "no args", errors.New("no args")
	}

	idx := 0
	if args[0] == "-s" {
		idx = 1
	}

	switch args[idx] {
	case "qdisc":
		return d.handleQdisc(args[idx+1:])
	case "filter":
		return d.handleFilter(args[idx+1:])
	default:
		return "", "unsupported", fmt.Errorf("unsupported command: %v", args)
	}
}

func (d *DemoExecutor) handleQdisc(args []string) (string, string, error) {
	switch args[0] {
	case "add":
		iface := args[2]
		if d.qdiscs[iface] {
			return "", "RTNETLINK answers: File exists", errors.New("exit status 2")
		}
		d.qdiscs[iface] = true
		return "", "", nil
	case "del":
		iface := args[2]
		if !d.qdiscs[iface] {
			return "", "RTNETLINK answers: No such file or directory", errors.New("exit status 2")
		}
		delete(d.qdiscs, iface)
		delete(d.filters, iface+":ingress")
		delete(d.filters, iface+":egress")
		return "", "", nil
	case "show":
		if len(args) >= 3 && args[1] == "dev" {
			iface := args[2]
			if d.qdiscs[iface] {
				return fmt.Sprintf("qdisc clsact ffff: dev %s parent ffff:fff1\n", iface), "", nil
			}
			return "", "", nil
		}
		var sb strings.Builder
		for iface := range d.qdiscs {
			fmt.Fprintf(&sb, "qdisc clsact ffff: dev %s parent ffff:fff1\n", iface)
		}
		return sb.String(), "", nil
	}
	return "", "unknown qdisc op", errors.New("unknown qdisc op")
}

func (d *DemoExecutor) handleFilter(args []string) (string, string, error) {
	op := args[0]
	iface := args[2]
	hook := args[3]
	key := iface + ":" + hook

	switch op {
	case "add":
		if !d.qdiscs[iface] {
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
		if d.filters[key] == nil {
			d.filters[key] = make(map[int]*demoFilter)
		}
		// Assign varied realistic traffic profiles per filter
		pps := int64(180 + d.rng.Intn(650))
		pktSize := int64(220 + d.rng.Intn(980))
		d.filters[key][pref] = &demoFilter{
			pref:       pref,
			handle:     handle,
			dstIntf:    dstIntf,
			packets:    pps * int64(5+d.rng.Intn(15)),
			bytes:      pps * pktSize * int64(5+d.rng.Intn(15)),
			ppsBase:    pps,
			avgPktSize: pktSize,
			lastTick:   time.Now(),
		}
		return "", "", nil

	case "del":
		var pref int
		for i := 4; i < len(args); i++ {
			if args[i] == "pref" {
				pref, _ = strconv.Atoi(args[i+1])
				i++
			}
		}
		if d.filters[key] == nil || d.filters[key][pref] == nil {
			return "", "RTNETLINK answers: No such file or directory", errors.New("exit status 2")
		}
		delete(d.filters[key], pref)
		return "", "", nil

	case "show":
		if !d.qdiscs[iface] {
			return "", "", nil
		}
		now := time.Now()
		var sb strings.Builder
		for _, flt := range d.filters[key] {
			// Advance simulated counters proportional to elapsed time
			elapsed := now.Sub(flt.lastTick).Seconds()
			if elapsed >= 0.2 {
				jitter := 0.7 + d.rng.Float64()*0.6
				dp := int64(float64(flt.ppsBase) * elapsed * jitter)
				if dp > 0 {
					flt.packets += dp
					flt.bytes += dp * flt.avgPktSize
					if d.rng.Intn(25) == 0 {
						flt.dropped++
					}
				}
				flt.lastTick = now
			}

			fmt.Fprintf(&sb, "filter protocol ip pref %d flower chain 0\n", flt.pref)
			fmt.Fprintf(&sb, "filter protocol ip pref %d flower chain 0 handle %s\n", flt.pref, flt.handle)
			fmt.Fprintf(&sb, "  eth_type ipv4\n")
			fmt.Fprintf(&sb, "  not_in_hw\n")
			fmt.Fprintf(&sb, "\taction order 1: mirred (Egress Mirror to device %s) continue\n", flt.dstIntf)
			fmt.Fprintf(&sb, "\tindex 1 ref 1 bind 1 installed 60 sec used 1 sec\n")
			fmt.Fprintf(&sb, "\tAction statistics:\n")
			fmt.Fprintf(&sb, "\tSent %d bytes %d pkt (dropped %d, overlimits 0 requeues 0)\n", flt.bytes, flt.packets, flt.dropped)
			fmt.Fprintf(&sb, "\tbacklog 0b 0p requeues 0\n")
		}
		return sb.String(), "", nil
	}
	return "", "unknown filter op", errors.New("unknown filter op")
}
