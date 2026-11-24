package manager

import (
	"fmt"
	"reflect"
	"sort"

	"tcbroker/pkg/config"
	"tcbroker/pkg/filter"
	"tcbroker/pkg/state"
	"tcbroker/pkg/tc"
)

const (
	// Pref ranges dedicated to tcbroker-managed filters.
	// Two generations allow safe blue/green swaps when rule ordering changes.
	Gen0Start = 20000
	Gen0End   = 24999
	Gen1Start = 25000
	Gen1End   = 29999

	DefaultHandle = "0x1"
)

// Manager orchestrates stateful, idempotent TC filter management.
type Manager struct {
	Runner    *tc.Runner
	StatePath string
}

// New creates a new Manager instance.
func New(runner *tc.Runner, statePath string) *Manager {
	return &Manager{
		Runner:    runner,
		StatePath: statePath,
	}
}

type ifaceHook struct {
	src  string
	hook string
}

// ApplyPlan describes the exact TC filter changes required to reconcile desired config.
type ApplyPlan struct {
	ConfigPath    string
	Ports         map[string]config.PortConfig
	Kept          []state.ManagedFilter
	ToAdd         []state.ManagedFilter
	ToDelete      []state.ManagedFilter
	NextFilters   []state.ManagedFilter
	RequiredSrcs  []string
	ManagedQdiscs map[string]state.ManagedQdisc
	DriftedCount  int
}

// HasChanges returns true if any filters will be added or deleted.
func (p *ApplyPlan) HasChanges() bool {
	return len(p.ToAdd) > 0 || len(p.ToDelete) > 0
}

// ApplyResult summarizes what an Apply call performed.
type ApplyResult struct {
	Plan          *ApplyPlan
	CreatedQdiscs []string
	RemovedQdiscs []string
	DryRun        bool
}

// DeleteResult summarizes what a Delete call performed.
type DeleteResult struct {
	DeletedFilters []state.ManagedFilter
	DeletedRules   []string
	RemovedQdiscs  []string
	RemainingRules int
	DryRun         bool
}

// RuleHealth represents the runtime health of a managed rule compared to live TC state.
type RuleHealth string

const (
	HealthActive  RuleHealth = "ACTIVE"
	HealthPartial RuleHealth = "PARTIAL"
	HealthMissing RuleHealth = "MISSING"
)

// FilterStatus reports runtime status and counters for a single managed TC filter.
type FilterStatus struct {
	Managed state.ManagedFilter `json:"managed"`
	Present bool                `json:"present"`
	Packets int64               `json:"packets"`
	Bytes   int64               `json:"bytes"`
	Dropped int64               `json:"dropped"`
}

// RuleStatus aggregates runtime status and counters for a single logical rule.
type RuleStatus struct {
	Name          string         `json:"name"`
	SrcIntf       string         `json:"src"`
	Hooks         []string       `json:"hooks"`
	Destinations  []string       `json:"destinations"`
	Health        RuleHealth     `json:"status"`
	ActiveFilters int            `json:"active_filters"`
	TotalFilters  int            `json:"total_filters"`
	Packets       int64          `json:"packets"`
	Bytes         int64          `json:"bytes"`
	Dropped       int64          `json:"dropped"`
	Filters       []FilterStatus `json:"filters"`
}

// StatusReport contains the full runtime status of all tcbroker-managed rules.
type StatusReport struct {
	StatePath  string                       `json:"state_path"`
	ConfigPath string                       `json:"config_path,omitempty"`
	Ports      map[string]config.PortConfig `json:"ports,omitempty"`
	Rules      []RuleStatus                 `json:"rules"`
}

// HasDrift returns true if any managed rule has missing filters in the kernel.
func (r *StatusReport) HasDrift() bool {
	for _, rule := range r.Rules {
		if rule.Health != HealthActive {
			return true
		}
	}
	return false
}

// expandConfig flattens declarative Rules into ordered desired ManagedFilters (without pref assigned).
func expandConfig(cfg *config.Config) []state.ManagedFilter {
	var out []state.ManagedFilter
	for _, rule := range cfg.Rules {
		hooks := rule.EffectiveHooks()
		filters := rule.EffectiveFilters()
		for _, hook := range hooks {
			for _, dst := range rule.Dst {
				rw := rule.EffectiveRewrite(dst)
				var rwCopy *filter.RewriteOptions
				if !rw.IsEmpty() {
					c := *rw
					rwCopy = &c
				}
				vlan := rule.EffectiveVLAN(dst)
				var vlanCopy *filter.VLANOptions
				if !vlan.IsEmpty() {
					c := *vlan
					vlanCopy = &c
				}
				rl := rule.EffectiveRateLimit(dst)
				var rlCopy *filter.RateLimitOptions
				if !rl.IsEmpty() {
					c := *rl
					rlCopy = &c
				}
				for _, f := range filters {
					out = append(out, state.ManagedFilter{
						RuleName:  rule.Name,
						SrcIntf:   rule.Src,
						Hook:      hook,
						DstIntf:   dst.Intf,
						Handle:    DefaultHandle,
						Filter:    f,
						Rewrite:   rwCopy,
						VLAN:      vlanCopy,
						RateLimit: rlCopy,
						SpecHash:  state.ComputeSpecHash(rule.Src, hook, dst.Intf, f, rwCopy, vlanCopy, rlCopy),
					})
				}
			}
		}
	}
	return out
}

// PlanApply computes the minimal safe set of additions and deletions needed to converge
// kernel TC state to cfg without disturbing filters owned by other tools.
func (m *Manager) PlanApply(cfg *config.Config, configPath string) (*ApplyPlan, error) {
	curState, err := state.Load(m.StatePath)
	if err != nil {
		return nil, err
	}

	desiredAll := expandConfig(cfg)

	desiredByGroup := make(map[ifaceHook][]state.ManagedFilter)
	var groupOrder []ifaceHook
	seenGroup := make(map[ifaceHook]bool)
	addGroup := func(k ifaceHook) {
		if !seenGroup[k] {
			seenGroup[k] = true
			groupOrder = append(groupOrder, k)
		}
	}

	for _, d := range desiredAll {
		k := ifaceHook{src: d.SrcIntf, hook: d.Hook}
		addGroup(k)
		desiredByGroup[k] = append(desiredByGroup[k], d)
	}

	stateByGroup := make(map[ifaceHook][]state.ManagedFilter)
	for _, s := range curState.Filters {
		k := ifaceHook{src: s.SrcIntf, hook: s.Hook}
		addGroup(k)
		stateByGroup[k] = append(stateByGroup[k], s)
	}

	plan := &ApplyPlan{
		ConfigPath:    configPath,
		Ports:         clonePorts(cfg.Ports),
		ManagedQdiscs: make(map[string]state.ManagedQdisc),
	}
	for k, v := range curState.ManagedQdiscs {
		plan.ManagedQdiscs[k] = v
	}

	requiredSrcSet := make(map[string]bool)
	for _, d := range desiredAll {
		if !requiredSrcSet[d.SrcIntf] {
			requiredSrcSet[d.SrcIntf] = true
			plan.RequiredSrcs = append(plan.RequiredSrcs, d.SrcIntf)
		}
	}

	for _, g := range groupOrder {
		desired := desiredByGroup[g]
		prev := stateByGroup[g]

		// Sort previous managed filters by pref to preserve actual kernel evaluation order.
		sort.Slice(prev, func(i, j int) bool {
			return prev[i].Pref < prev[j].Pref
		})

		// Inspect live kernel filters to avoid colliding with external tools and detect drift.
		liveFilters, err := m.Runner.ListParsedFilters(g.src, g.hook)
		if err != nil {
			return nil, err
		}
		livePrefs := make(map[int]bool, len(liveFilters))
		liveByPrefHandle := make(map[string]bool, len(liveFilters))
		for _, lf := range liveFilters {
			livePrefs[lf.Priority] = true
			liveByPrefHandle[prefHandleKey(lf.Priority, lf.Handle)] = true
		}

		// Filter previous managed entries to those actually still present in kernel.
		var alive []state.ManagedFilter
		for _, p := range prev {
			if liveByPrefHandle[prefHandleKey(p.Pref, p.Handle)] || livePrefs[p.Pref] {
				alive = append(alive, p)
			} else {
				plan.DriftedCount++
			}
		}

		// Case 0: Group removed completely from desired config.
		if len(desired) == 0 {
			plan.ToDelete = append(plan.ToDelete, alive...)
			continue
		}

		// Case 1: Exact match in order and spec -> idempotent no-op (preserve stats!).
		if equalManagedLists(alive, desired) {
			plan.Kept = append(plan.Kept, alive...)
			plan.NextFilters = append(plan.NextFilters, alive...)
			continue
		}

		// Case 2: Pure append at the end of existing alive filters -> keep existing stats, append new prefs.
		if len(desired) > len(alive) && len(alive) > 0 && equalManagedLists(alive, desired[:len(alive)]) {
			plan.Kept = append(plan.Kept, alive...)
			plan.NextFilters = append(plan.NextFilters, alive...)

			startPref := alive[len(alive)-1].Pref + 1
			rangeEnd := Gen0End
			if startPref >= Gen1Start {
				rangeEnd = Gen1End
			}
			newFilters, allocErr := allocatePrefs(desired[len(alive):], startPref, rangeEnd, livePrefs)
			if allocErr == nil {
				plan.ToAdd = append(plan.ToAdd, newFilters...)
				plan.NextFilters = append(plan.NextFilters, newFilters...)
				continue
			}
			// Fallback to generation swap if current range is exhausted.
		}

		// Case 3: Pure removal of a subset while preserving order -> keep surviving stats, delete removed prefs.
		if len(desired) < len(alive) {
			if kept, removed, ok := matchSubsequence(alive, desired); ok {
				plan.Kept = append(plan.Kept, kept...)
				plan.ToDelete = append(plan.ToDelete, removed...)
				plan.NextFilters = append(plan.NextFilters, kept...)
				continue
			}
		}

		// Case 4: Modification, reorder, or drift recovery -> blue/green generation swap.
		rangeStart, rangeEnd := selectNextGeneration(alive)
		newFilters, allocErr := allocatePrefs(desired, rangeStart, rangeEnd, livePrefs)
		if allocErr != nil {
			return nil, fmt.Errorf("failed to allocate TC priorities on %s (%s): %w", g.src, g.hook, allocErr)
		}
		plan.ToAdd = append(plan.ToAdd, newFilters...)
		plan.ToDelete = append(plan.ToDelete, alive...)
		plan.NextFilters = append(plan.NextFilters, newFilters...)
	}

	return plan, nil
}

// Apply reconciles kernel TC rules with cfg, rolling back newly added filters on failure.
func (m *Manager) Apply(cfg *config.Config, configPath string) (*ApplyResult, error) {
	plan, err := m.PlanApply(cfg, configPath)
	if err != nil {
		return nil, err
	}

	result := &ApplyResult{
		Plan:   plan,
		DryRun: m.Runner.DryRun,
	}

	if !plan.HasChanges() && plan.DriftedCount == 0 {
		// Still ensure state reflects updated configPath or port labels even when TC filters didn't change.
		if !m.Runner.DryRun {
			curState, loadErr := state.Load(m.StatePath)
			if loadErr == nil && (curState.ConfigPath != configPath || !reflect.DeepEqual(curState.Ports, plan.Ports)) {
				curState.ConfigPath = configPath
				curState.Ports = plan.Ports
				_ = state.Save(m.StatePath, curState)
			}
		}
		return result, nil
	}

	// Step 1: Ensure clsact qdisc on all required source interfaces.
	newlyCreatedQdiscs := make(map[string]bool)
	for _, src := range plan.RequiredSrcs {
		created, qErr := m.Runner.EnsureClsactQdisc(src)
		if qErr != nil {
			return nil, qErr
		}
		if created {
			newlyCreatedQdiscs[src] = true
			result.CreatedQdiscs = append(result.CreatedQdiscs, src)
			plan.ManagedQdiscs[src] = state.ManagedQdisc{CreatedByTCBroker: true}
		} else if _, exists := plan.ManagedQdiscs[src]; !exists {
			// Already existed before tcbroker touched this interface; never delete it on cleanup.
			plan.ManagedQdiscs[src] = state.ManagedQdisc{CreatedByTCBroker: false}
		}
	}

	// Step 2: Add new filters (with automatic rollback on failure).
	var addedSoFar []state.ManagedFilter
	for _, f := range plan.ToAdd {
		spec := filter.BuildSpec{
			SrcIntf:   f.SrcIntf,
			Hook:      f.Hook,
			Pref:      f.Pref,
			Handle:    f.Handle,
			DstIntf:   f.DstIntf,
			Filter:    f.Filter,
			Rewrite:   f.Rewrite,
			VLAN:      f.VLAN,
			RateLimit: f.RateLimit,
		}
		if addErr := m.Runner.AddFilter(spec); addErr != nil {
			// Roll back newly added filters in reverse order.
			if !m.Runner.DryRun {
				for i := len(addedSoFar) - 1; i >= 0; i-- {
					rf := addedSoFar[i]
					_ = m.Runner.DeleteFilter(rf.SrcIntf, rf.Hook, rf.Pref, rf.Handle)
				}
				for iface := range newlyCreatedQdiscs {
					_ = m.cleanupQdiscIfSafe(iface, true)
				}
			}
			return nil, fmt.Errorf("apply failed (rolled back %d newly added filter(s)): %w", len(addedSoFar), addErr)
		}
		addedSoFar = append(addedSoFar, f)
	}

	// Step 3: Remove obsolete filters.
	for _, f := range plan.ToDelete {
		if delErr := m.Runner.DeleteFilter(f.SrcIntf, f.Hook, f.Pref, f.Handle); delErr != nil {
			return nil, fmt.Errorf("new filters applied, but failed to remove obsolete filter (rule %q, %s %s pref %d): %w",
				f.RuleName, f.SrcIntf, f.Hook, f.Pref, delErr)
		}
	}

	if m.Runner.DryRun {
		return result, nil
	}

	// Step 4: Clean up clsact qdiscs on interfaces that no longer have any tcbroker filters
	// ONLY when tcbroker created the qdisc AND no other filters remain.
	activeSrcs := make(map[string]bool)
	for _, f := range plan.NextFilters {
		activeSrcs[f.SrcIntf] = true
	}
	for iface, qinfo := range plan.ManagedQdiscs {
		if activeSrcs[iface] {
			continue
		}
		if qinfo.CreatedByTCBroker {
			removed, cleanErr := m.cleanupQdiscIfEmpty(iface)
			if cleanErr == nil && removed {
				result.RemovedQdiscs = append(result.RemovedQdiscs, iface)
			}
		}
		delete(plan.ManagedQdiscs, iface)
	}

	// Step 5: Persist updated state atomically.
	nextState := &state.State{
		ConfigPath:    configPath,
		Ports:         plan.Ports,
		ManagedQdiscs: plan.ManagedQdiscs,
		Filters:       plan.NextFilters,
	}
	if err := state.Save(m.StatePath, nextState); err != nil {
		return nil, fmt.Errorf("TC rules applied, but failed to save state: %w", err)
	}

	return result, nil
}

// Delete removes either all tcbroker-managed rules (deleteAll=true) or specific rules by name.
// External TC filters and pre-existing clsact qdiscs are never touched.
func (m *Manager) Delete(ruleNames []string, deleteAll bool) (*DeleteResult, error) {
	curState, err := state.Load(m.StatePath)
	if err != nil {
		return nil, err
	}

	res := &DeleteResult{
		DryRun: m.Runner.DryRun,
	}

	if len(curState.Filters) == 0 {
		if !deleteAll && len(ruleNames) > 0 {
			return nil, fmt.Errorf("no active tcbroker rules found (rule %q does not exist)", ruleNames[0])
		}
		return res, nil
	}

	targetSet := make(map[string]bool)
	if !deleteAll {
		existingRules := make(map[string]bool)
		for _, f := range curState.Filters {
			existingRules[f.RuleName] = true
		}
		for _, name := range ruleNames {
			if !existingRules[name] {
				return nil, fmt.Errorf("rule %q not found in active state (active rules: %v)", name, curState.RuleNames())
			}
			targetSet[name] = true
		}
	}

	var toDelete, remaining []state.ManagedFilter
	deletedRuleSet := make(map[string]bool)
	affectedSrcs := make(map[string]bool)

	for _, f := range curState.Filters {
		if deleteAll || targetSet[f.RuleName] {
			toDelete = append(toDelete, f)
			affectedSrcs[f.SrcIntf] = true
			if !deletedRuleSet[f.RuleName] {
				deletedRuleSet[f.RuleName] = true
				res.DeletedRules = append(res.DeletedRules, f.RuleName)
			}
		} else {
			remaining = append(remaining, f)
		}
	}

	for _, f := range toDelete {
		if err := m.Runner.DeleteFilter(f.SrcIntf, f.Hook, f.Pref, f.Handle); err != nil {
			return nil, err
		}
	}
	res.DeletedFilters = toDelete

	remainingSrcs := make(map[string]bool)
	remainingRuleSet := make(map[string]bool)
	for _, f := range remaining {
		remainingSrcs[f.SrcIntf] = true
		remainingRuleSet[f.RuleName] = true
	}
	res.RemainingRules = len(remainingRuleSet)

	if m.Runner.DryRun {
		return res, nil
	}

	// Safely remove clsact qdisc only if tcbroker created it and no filters remain on either hook.
	for iface := range affectedSrcs {
		if remainingSrcs[iface] {
			continue
		}
		qinfo := curState.ManagedQdiscs[iface]
		if qinfo.CreatedByTCBroker {
			removed, cleanErr := m.cleanupQdiscIfEmpty(iface)
			if cleanErr == nil && removed {
				res.RemovedQdiscs = append(res.RemovedQdiscs, iface)
			}
		}
		delete(curState.ManagedQdiscs, iface)
	}

	if len(remaining) == 0 {
		if err := state.Remove(m.StatePath); err != nil {
			return nil, err
		}
	} else {
		curState.Filters = remaining
		if err := state.Save(m.StatePath, curState); err != nil {
			return nil, err
		}
	}

	return res, nil
}

// Status queries live kernel TC statistics and correlates them accurately by (pref, handle)
// against tcbroker's persisted state.
func (m *Manager) Status(ruleNameFilter string) (*StatusReport, error) {
	curState, err := state.Load(m.StatePath)
	if err != nil {
		return nil, err
	}

	report := &StatusReport{
		StatePath:  m.StatePath,
		ConfigPath: curState.ConfigPath,
		Ports:      clonePorts(curState.Ports),
		Rules:      []RuleStatus{},
	}

	if len(curState.Filters) == 0 {
		if ruleNameFilter != "" {
			return nil, fmt.Errorf("rule %q not found (no active tcbroker rules)", ruleNameFilter)
		}
		return report, nil
	}

	// Query each unique (src, hook) once.
	liveIndex := make(map[ifaceHook]map[int]tc.FilterStats)
	for _, f := range curState.Filters {
		if ruleNameFilter != "" && f.RuleName != ruleNameFilter {
			continue
		}
		k := ifaceHook{src: f.SrcIntf, hook: f.Hook}
		if _, ok := liveIndex[k]; ok {
			continue
		}
		parsed, err := m.Runner.ListParsedFiltersWithStats(f.SrcIntf, f.Hook)
		if err != nil {
			return nil, err
		}
		byPref := make(map[int]tc.FilterStats, len(parsed))
		for _, pf := range parsed {
			byPref[pf.Priority] = pf
		}
		liveIndex[k] = byPref
	}

	// Aggregate per rule, preserving order.
	ruleOrder := []string{}
	ruleMap := make(map[string]*RuleStatus)
	hookSeen := make(map[string]map[string]bool)
	dstSeen := make(map[string]map[string]bool)

	for _, f := range curState.Filters {
		if ruleNameFilter != "" && f.RuleName != ruleNameFilter {
			continue
		}
		rs, exists := ruleMap[f.RuleName]
		if !exists {
			rs = &RuleStatus{
				Name:    f.RuleName,
				SrcIntf: f.SrcIntf,
			}
			ruleMap[f.RuleName] = rs
			ruleOrder = append(ruleOrder, f.RuleName)
			hookSeen[f.RuleName] = make(map[string]bool)
			dstSeen[f.RuleName] = make(map[string]bool)
		}
		if !hookSeen[f.RuleName][f.Hook] {
			hookSeen[f.RuleName][f.Hook] = true
			rs.Hooks = append(rs.Hooks, f.Hook)
		}
		if !dstSeen[f.RuleName][f.DstIntf] {
			dstSeen[f.RuleName][f.DstIntf] = true
			rs.Destinations = append(rs.Destinations, f.DstIntf)
		}

		fs := FilterStatus{Managed: f}
		k := ifaceHook{src: f.SrcIntf, hook: f.Hook}
		if liveFilter, ok := liveIndex[k][f.Pref]; ok {
			fs.Present = true
			rs.ActiveFilters++
			if mirred := liveFilter.FindMirredAction(f.DstIntf); mirred != nil {
				fs.Packets = mirred.Packets
				fs.Bytes = mirred.Bytes
				fs.Dropped = mirred.Dropped
			}
		}

		rs.TotalFilters++
		rs.Packets += fs.Packets
		rs.Bytes += fs.Bytes
		rs.Dropped += fs.Dropped
		rs.Filters = append(rs.Filters, fs)
	}

	if ruleNameFilter != "" && len(ruleOrder) == 0 {
		return nil, fmt.Errorf("rule %q not found (active rules: %v)", ruleNameFilter, curState.RuleNames())
	}

	for _, name := range ruleOrder {
		rs := ruleMap[name]
		switch rs.ActiveFilters {
		case rs.TotalFilters:
			rs.Health = HealthActive
		case 0:
			rs.Health = HealthMissing
		default:
			rs.Health = HealthPartial
		}
		report.Rules = append(report.Rules, *rs)
	}

	return report, nil
}

func clonePorts(src map[string]config.PortConfig) map[string]config.PortConfig {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]config.PortConfig, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (m *Manager) cleanupQdiscIfSafe(iface string, createdByTCBroker bool) error {
	if !createdByTCBroker {
		return nil
	}
	_, err := m.cleanupQdiscIfEmpty(iface)
	return err
}

func (m *Manager) cleanupQdiscIfEmpty(iface string) (bool, error) {
	ingress, err := m.Runner.ListParsedFilters(iface, "ingress")
	if err != nil {
		return false, err
	}
	egress, err := m.Runner.ListParsedFilters(iface, "egress")
	if err != nil {
		return false, err
	}
	if len(ingress) > 0 || len(egress) > 0 {
		// External TC filters still exist; keep clsact intact.
		return false, nil
	}
	if err := m.Runner.DeleteClsactQdisc(iface); err != nil {
		return false, err
	}
	return true, nil
}

func prefHandleKey(pref int, handle string) string {
	return fmt.Sprintf("%d:%s", pref, handle)
}

func equalManagedLists(a, b []state.ManagedFilter) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].RuleName != b[i].RuleName || a[i].SpecHash != b[i].SpecHash {
			return false
		}
	}
	return true
}

// matchSubsequence checks whether desired is an order-preserving subsequence of alive.
func matchSubsequence(alive, desired []state.ManagedFilter) (kept []state.ManagedFilter, removed []state.ManagedFilter, ok bool) {
	j := 0
	for _, item := range alive {
		if j < len(desired) && item.RuleName == desired[j].RuleName && item.SpecHash == desired[j].SpecHash {
			kept = append(kept, item)
			j++
		} else {
			removed = append(removed, item)
		}
	}
	return kept, removed, j == len(desired)
}

func selectNextGeneration(alive []state.ManagedFilter) (int, int) {
	inGen0 := 0
	inGen1 := 0
	for _, f := range alive {
		if f.Pref >= Gen0Start && f.Pref <= Gen0End {
			inGen0++
		} else if f.Pref >= Gen1Start && f.Pref <= Gen1End {
			inGen1++
		}
	}
	if inGen0 > inGen1 {
		return Gen1Start, Gen1End
	}
	return Gen0Start, Gen0End
}

func allocatePrefs(items []state.ManagedFilter, start, end int, occupied map[int]bool) ([]state.ManagedFilter, error) {
	out := make([]state.ManagedFilter, 0, len(items))
	nextPref := start
	for _, item := range items {
		for nextPref <= end && occupied[nextPref] {
			nextPref++
		}
		if nextPref > end {
			return nil, fmt.Errorf("exhausted available priorities in range %d-%d", start, end)
		}
		assigned := item
		assigned.Pref = nextPref
		if assigned.Handle == "" {
			assigned.Handle = DefaultHandle
		}
		out = append(out, assigned)
		occupied[nextPref] = true
		nextPref++
	}
	return out, nil
}
