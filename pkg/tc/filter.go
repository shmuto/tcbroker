package tc

import (
	"fmt"
	"strings"

	"tcbroker/pkg/filter"
)

// AddFilter installs a single TC flower filter according to the given BuildSpec.
func (r *Runner) AddFilter(spec filter.BuildSpec) error {
	args := filter.BuildTCArgs(spec)
	_, stderr, err := r.RunWrite(args...)
	if err != nil {
		return fmt.Errorf("failed to add filter on %s (%s, pref %d): %w (stderr: %s)",
			spec.SrcIntf, spec.Hook, spec.Pref, err, strings.TrimSpace(stderr))
	}
	return nil
}

// DeleteFilter removes a single TC filter identified by interface, hook, priority, and optional handle.
// If the filter or device is already absent, it succeeds idempotently.
func (r *Runner) DeleteFilter(srcIntf, hook string, pref int, handle string) error {
	args := filter.BuildDeleteArgs(srcIntf, hook, pref, handle)
	_, stderr, err := r.RunWrite(args...)
	if err != nil {
		if isNotFoundTCError(stderr) {
			return nil
		}
		return fmt.Errorf("failed to delete filter on %s (%s, pref %d): %w (stderr: %s)",
			srcIntf, hook, pref, err, strings.TrimSpace(stderr))
	}
	return nil
}

// ListParsedFilters returns parsed filters currently installed on (iface, hook).
// If the interface has no clsact qdisc yet, an empty slice is returned without error.
func (r *Runner) ListParsedFilters(iface, hook string) ([]FilterStats, error) {
	out, stderr, err := r.RunRead("filter", "show", "dev", iface, hook)
	if err != nil {
		if isNotFoundTCError(stderr) {
			return []FilterStats{}, nil
		}
		return nil, fmt.Errorf("failed to list filters for %s (%s): %w (stderr: %s)", iface, hook, err, strings.TrimSpace(stderr))
	}
	return ParseFilterStats(out)
}

// ListParsedFiltersWithStats returns parsed filters with packet/byte counters on (iface, hook).
func (r *Runner) ListParsedFiltersWithStats(iface, hook string) ([]FilterStats, error) {
	out, stderr, err := r.RunRead("-s", "filter", "show", "dev", iface, hook)
	if err != nil {
		if isNotFoundTCError(stderr) {
			return []FilterStats{}, nil
		}
		return nil, fmt.Errorf("failed to list filter stats for %s (%s): %w (stderr: %s)", iface, hook, err, strings.TrimSpace(stderr))
	}
	return ParseFilterStats(out)
}
