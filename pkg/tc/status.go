package tc

import (
	"fmt"
	"strings"
)

// ListQdiscs returns a list of qdiscs for the specified interface.
func (r *Runner) ListQdiscs(iface string) (string, error) {
	stdout, stderr, err := r.RunRead("qdisc", "show", "dev", iface)
	if err != nil {
		return "", fmt.Errorf("failed to list qdiscs for %s: %w (stderr: %s)", iface, err, strings.TrimSpace(stderr))
	}
	return stdout, nil
}

// ListFilters returns the raw output of `tc filter show dev <iface> <hook>`.
func (r *Runner) ListFilters(iface, hook string) (string, error) {
	stdout, stderr, err := r.RunRead("filter", "show", "dev", iface, hook)
	if err != nil {
		if isNotFoundTCError(stderr) {
			return "", nil
		}
		return "", fmt.Errorf("failed to list filters for %s (%s): %w (stderr: %s)", iface, hook, err, strings.TrimSpace(stderr))
	}
	return stdout, nil
}

// ListFiltersWithStats returns the raw output of `tc -s filter show dev <iface> <hook>`.
func (r *Runner) ListFiltersWithStats(iface, hook string) (string, error) {
	stdout, stderr, err := r.RunRead("-s", "filter", "show", "dev", iface, hook)
	if err != nil {
		if isNotFoundTCError(stderr) {
			return "", nil
		}
		return "", fmt.Errorf("failed to list filters with stats for %s (%s): %w (stderr: %s)", iface, hook, err, strings.TrimSpace(stderr))
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

// GetAllInterfaces returns the raw output of `tc qdisc show`.
func (r *Runner) GetAllInterfaces() (string, error) {
	stdout, stderr, err := r.RunRead("qdisc", "show")
	if err != nil {
		return "", fmt.Errorf("failed to list all qdiscs: %w (stderr: %s)", err, strings.TrimSpace(stderr))
	}
	return stdout, nil
}
