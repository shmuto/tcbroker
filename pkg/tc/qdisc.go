package tc

import (
	"fmt"
	"strings"
)

// EnsureClsactQdisc ensures the clsact qdisc is attached to the specified network interface.
// The clsact qdisc is a modern replacement for the ingress qdisc and can handle both
// ingress and egress hooks.
// Command: `tc qdisc add dev <iface> clsact`
func (r *Runner) EnsureClsactQdisc(iface string) error {
	// Using "replace" is safer as it will create or update the qdisc.
	// If it already exists, "add" would fail. "replace" avoids this.
	// However, for simplicity and sticking to the original plan, we use "add"
	// and handle the error. A simple "add" is often sufficient.
	_, stderr, err := r.Run("qdisc", "add", "dev", iface, "clsact")
	if err != nil {
		// Ignore "File exists" error, which means the qdisc is already there.
		if strings.Contains(stderr, "File exists") {
			return nil
		}
		return fmt.Errorf("failed to add clsact qdisc to %s: %w, stderr: %s", iface, err, stderr)
	}
	return nil
}

// DeleteClsactQdisc deletes the clsact qdisc from the specified network interface.
// Command: `tc qdisc del dev <iface> clsact`
func (r *Runner) DeleteClsactQdisc(iface string) error {
	_, stderr, err := r.Run("qdisc", "del", "dev", iface, "clsact")
	if err != nil {
		// Ignore errors that indicate the qdisc doesn't exist or is already gone
		if strings.Contains(stderr, "Cannot find device") ||
			strings.Contains(stderr, "No such file or directory") ||
			strings.Contains(stderr, "Invalid handle") ||
			strings.Contains(stderr, "RTNETLINK answers: No such file or directory") {
			return nil
		}
		return fmt.Errorf("failed to delete clsact qdisc from %s: %w, stderr: %s", iface, err, stderr)
	}
	return nil
}
