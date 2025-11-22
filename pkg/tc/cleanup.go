package tc

import (
	"fmt"
	"tcbroker/pkg/config"
)

// Cleanup removes all tc configurations (qdisc and filters) for the interfaces
// specified in the given configuration.
// This is done by deleting the clsact qdisc from each interface, which implicitly
// removes all attached filters.
func (r *Runner) Cleanup(cfg *config.Config) error {
	// Collect unique source interfaces from rules
	interfaceMap := make(map[string]bool)
	for _, rule := range cfg.Rules {
		interfaceMap[rule.SrcIntf] = true
	}

	// Delete clsact qdisc from each source interface
	for ifaceName := range interfaceMap {
		if err := r.DeleteClsactQdisc(ifaceName); err != nil {
			return fmt.Errorf("failed to cleanup %s: %w", ifaceName, err)
		}
	}
	return nil
}
