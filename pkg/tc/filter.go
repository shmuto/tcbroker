package tc

import (
	"fmt"

	"tcbroker/pkg/config"
	"tcbroker/pkg/filter"
)

// AddMirrorFilter adds a new filter to the given interface that mirrors traffic
// to the target interface. It attaches the filter to the appropriate hook (ingress/egress)
// on the clsact qdisc. Optionally supports packet rewriting.
// Command: `tc filter add dev <iface> <hook> protocol <proto> flower <matchers> action mirred egress mirror dev <target>`
func (r *Runner) AddMirrorFilter(ifaceName, direction, target string, f filter.Filter, rewrite *config.RewriteOptions) error {
	directions := []string{}
	switch direction {
	case "ingress":
		directions = append(directions, "ingress")
	case "egress":
		directions = append(directions, "egress")
	case "both":
		directions = append(directions, "ingress", "egress")
	default:
		return fmt.Errorf("invalid direction '%s'", direction)
	}

	for _, hook := range directions {
		var args []string

		// Use BuildTCArgsWithRewrite if rewrite options are provided
		if rewrite != nil {
			// Convert config.RewriteOptions to filter.RewriteOptions
			filterRewrite := &filter.RewriteOptions{
				DstMAC: rewrite.DstMAC,
				SrcMAC: rewrite.SrcMAC,
				DstIP:  rewrite.DstIP,
				SrcIP:  rewrite.SrcIP,
			}
			args = filter.BuildTCArgsWithRewrite(ifaceName, hook, target, f, filterRewrite)
		} else {
			args = filter.BuildTCArgs(ifaceName, hook, target, f)
		}

		_, stderr, err := r.Run(args...)
		if err != nil {
			return fmt.Errorf("failed to add mirror filter to %s (%s): %w, stderr: %s", ifaceName, hook, err, stderr)
		}
	}
	return nil
}
