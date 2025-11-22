package tc

import (
	"fmt"
	"strings"
)

// ListQdiscs returns a list of qdiscs for the specified interface.
// Command: `tc qdisc show dev <iface>`
func (r *Runner) ListQdiscs(iface string) (string, error) {
	stdout, stderr, err := r.Run("qdisc", "show", "dev", iface)
	if err != nil {
		return "", fmt.Errorf("failed to list qdiscs for %s: %w, stderr: %s", iface, err, stderr)
	}
	return stdout, nil
}

// ListFilters returns a list of filters for the specified interface and hook (ingress/egress).
// Command: `tc filter show dev <iface> <hook>`
func (r *Runner) ListFilters(iface, hook string) (string, error) {
	stdout, stderr, err := r.Run("filter", "show", "dev", iface, hook)
	if err != nil {
		return "", fmt.Errorf("failed to list filters for %s (%s): %w, stderr: %s", iface, hook, err, stderr)
	}
	return stdout, nil
}

// ListFiltersWithStats returns a list of filters with statistics for the specified interface and hook.
// Command: `tc -s filter show dev <iface> <hook>`
func (r *Runner) ListFiltersWithStats(iface, hook string) (string, error) {
	stdout, stderr, err := r.Run("-s", "filter", "show", "dev", iface, hook)
	if err != nil {
		return "", fmt.Errorf("failed to list filters with stats for %s (%s): %w, stderr: %s", iface, hook, err, stderr)
	}
	return stdout, nil
}

// HasClsactQdisc checks if the specified interface has a clsact qdisc attached.
func (r *Runner) HasClsactQdisc(iface string) (bool, error) {
	qdiscs, err := r.ListQdiscs(iface)
	if err != nil {
		return false, err
	}
	return strings.Contains(qdiscs, "clsact"), nil
}

// GetAllInterfaces returns a list of all network interfaces that have tc rules.
// This is a simple implementation that could be enhanced later.
// Command: `tc qdisc show`
func (r *Runner) GetAllInterfaces() (string, error) {
	stdout, stderr, err := r.Run("qdisc", "show")
	if err != nil {
		return "", fmt.Errorf("failed to list all qdiscs: %w, stderr: %s", err, stderr)
	}
	return stdout, nil
}
