package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tcbroker/pkg/config"
	"tcbroker/pkg/filter"
)

const (
	// CurrentStateVersion is the schema version of the state JSON file.
	CurrentStateVersion = 1

	// DefaultStatePath is the default location for tcbroker runtime state.
	DefaultStatePath = "/run/tcbroker/state.json"

	// EnvStateFile overrides DefaultStatePath when set.
	EnvStateFile = "TCBROKER_STATE_FILE"
)

// ResolveStatePath resolves the state file path from explicit flag, env var, or default.
func ResolveStatePath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	if env := os.Getenv(EnvStateFile); env != "" {
		return env
	}
	return DefaultStatePath
}

// State persists the TC resources actively managed by tcbroker.
type State struct {
	Version       int                          `json:"version"`
	ConfigPath    string                       `json:"config_path,omitempty"`
	UpdatedAt     time.Time                    `json:"updated_at"`
	Ports         map[string]config.PortConfig `json:"ports,omitempty"`
	ManagedQdiscs map[string]ManagedQdisc      `json:"managed_qdiscs"`
	Filters       []ManagedFilter              `json:"filters"`
}

// ManagedQdisc tracks whether tcbroker originally created a clsact qdisc on an interface.
type ManagedQdisc struct {
	CreatedByTCBroker bool `json:"created_by_tcbroker"`
}

// ManagedFilter represents a single TC flower filter installed and owned by tcbroker.
type ManagedFilter struct {
	RuleName  string                   `json:"rule_name"`
	SrcIntf   string                   `json:"src_intf"`
	Hook      string                   `json:"hook"` // "ingress" or "egress"
	DstIntf   string                   `json:"dst_intf"`
	Pref      int                      `json:"pref"`
	Handle    string                   `json:"handle"`
	Filter    filter.Filter            `json:"filter"`
	Rewrite   *filter.RewriteOptions   `json:"rewrite,omitempty"`
	VLAN      *filter.VLANOptions      `json:"vlan,omitempty"`
	RateLimit *filter.RateLimitOptions `json:"rate_limit,omitempty"`
	SpecHash  string                   `json:"spec_hash"`
}

// NewEmpty creates an initialized empty State.
func NewEmpty() *State {
	return &State{
		Version:       CurrentStateVersion,
		Ports:         make(map[string]config.PortConfig),
		ManagedQdiscs: make(map[string]ManagedQdisc),
		Filters:       []ManagedFilter{},
	}
}

// ComputeSpecHash deterministically hashes the functional specification of a TC filter
// (excluding pref/handle and rule name) so unchanged rules can be detected cleanly.
func ComputeSpecHash(srcIntf, hook, dstIntf string, f filter.Filter, rw *filter.RewriteOptions, vlan *filter.VLANOptions, rl *filter.RateLimitOptions) string {
	type hashInput struct {
		SrcIntf   string                   `json:"src_intf"`
		Hook      string                   `json:"hook"`
		DstIntf   string                   `json:"dst_intf"`
		Filter    filter.Filter            `json:"filter"`
		Rewrite   *filter.RewriteOptions   `json:"rewrite,omitempty"`
		VLAN      *filter.VLANOptions      `json:"vlan,omitempty"`
		RateLimit *filter.RateLimitOptions `json:"rate_limit,omitempty"`
	}
	var normRW *filter.RewriteOptions
	if !rw.IsEmpty() {
		normRW = rw
	}
	var normVLAN *filter.VLANOptions
	if !vlan.IsEmpty() {
		normVLAN = vlan
	}
	var normRL *filter.RateLimitOptions
	if !rl.IsEmpty() {
		normRL = &filter.RateLimitOptions{
			Rate:  rl.Rate,
			Burst: rl.EffectiveBurst(),
		}
	}
	raw, _ := json.Marshal(hashInput{
		SrcIntf:   srcIntf,
		Hook:      hook,
		DstIntf:   dstIntf,
		Filter:    f,
		Rewrite:   normRW,
		VLAN:      normVLAN,
		RateLimit: normRL,
	})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

// Load reads a State from disk. If the file does not exist, an empty State is returned.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NewEmpty(), nil
		}
		return nil, fmt.Errorf("failed to read state file %s: %w", path, err)
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("failed to parse state file %s: %w", path, err)
	}
	if st.Ports == nil {
		st.Ports = make(map[string]config.PortConfig)
	}
	if st.ManagedQdiscs == nil {
		st.ManagedQdiscs = make(map[string]ManagedQdisc)
	}
	if st.Filters == nil {
		st.Filters = []ManagedFilter{}
	}
	return &st, nil
}

// Save atomically writes State to disk with 0644 permissions so non-root users can run `status`.
func Save(path string, st *State) error {
	if st == nil {
		st = NewEmpty()
	}
	st.Version = CurrentStateVersion
	st.UpdatedAt = time.Now().UTC()
	if st.ManagedQdiscs == nil {
		st.ManagedQdiscs = make(map[string]ManagedQdisc)
	}
	if st.Filters == nil {
		st.Filters = []ManagedFilter{}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize state: %w", err)
	}
	data = append(data, '\n')

	tmpFile, err := os.CreateTemp(dir, ".tcbroker-state-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp state file: %w", err)
	}
	tmpPath := tmpFile.Name()

	cleanupTmp := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}

	if _, err := tmpFile.Write(data); err != nil {
		cleanupTmp()
		return fmt.Errorf("failed to write temp state file: %w", err)
	}
	if err := tmpFile.Chmod(0644); err != nil {
		cleanupTmp()
		return fmt.Errorf("failed to chmod temp state file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp state file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to commit state file %s: %w", path, err)
	}
	return nil
}

// Remove deletes the state file if present.
func Remove(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove state file %s: %w", path, err)
	}
	return nil
}

// RuleNames returns unique rule names currently present in the state, preserving order.
func (s *State) RuleNames() []string {
	seen := make(map[string]bool)
	var names []string
	for _, f := range s.Filters {
		if !seen[f.RuleName] {
			seen[f.RuleName] = true
			names = append(names, f.RuleName)
		}
	}
	return names
}
