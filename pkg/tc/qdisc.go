package tc

import (
	"fmt"
	"strings"
)

// EnsureClsactQdisc ensures the clsact qdisc is attached to the specified interface.
// It returns created=true only when tcbroker newly added the qdisc (not when it already existed).
func (r *Runner) EnsureClsactQdisc(iface string) (bool, error) {
	if r.DryRun {
		has, err := r.HasClsactQdisc(iface)
		if err == nil && has {
			return false, nil
		}
		_, _, _ = r.RunWrite("qdisc", "add", "dev", iface, "clsact")
		return true, nil
	}

	_, stderr, err := r.RunWrite("qdisc", "add", "dev", iface, "clsact")
	if err != nil {
		if strings.Contains(stderr, "File exists") {
			return false, nil
		}
		return false, fmt.Errorf("failed to add clsact qdisc to %s: %w (stderr: %s)", iface, err, strings.TrimSpace(stderr))
	}
	return true, nil
}

// DeleteClsactQdisc deletes the clsact qdisc from the specified network interface.
func (r *Runner) DeleteClsactQdisc(iface string) error {
	_, stderr, err := r.RunWrite("qdisc", "del", "dev", iface, "clsact")
	if err != nil {
		if isNotFoundTCError(stderr) {
			return nil
		}
		return fmt.Errorf("failed to delete clsact qdisc from %s: %w (stderr: %s)", iface, err, strings.TrimSpace(stderr))
	}
	return nil
}

func isNotFoundTCError(stderr string) bool {
	return strings.Contains(stderr, "Cannot find device") ||
		strings.Contains(stderr, "No such file or directory") ||
		strings.Contains(stderr, "Invalid handle") ||
		strings.Contains(stderr, "Cannot find filter") ||
		strings.Contains(stderr, "Filter with specified priority/protocol not found") ||
		strings.Contains(stderr, "Not found")
}
