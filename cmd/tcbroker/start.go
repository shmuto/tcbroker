package main

import (
	"fmt"
	"net"
	"os"

	"github.com/spf13/cobra"
	"tcbroker/pkg/config"
	"tcbroker/pkg/tc"
)

var (
	debug  bool
	dryRun bool
	force  bool
)

var startCmd = &cobra.Command{
	Use:   "start [config-file]",
	Short: "Starts the packet mirroring based on a config file.",
	Long: `Reads the given YAML configuration file, validates it, and applies the
	tc rules to start mirroring packets. This command requires root privileges.`,
	Args: cobra.ExactArgs(1),
	Run:  start,
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug mode to print tc commands")
	startCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Enable dry-run mode to print tc commands without executing them")
	startCmd.Flags().BoolVar(&force, "force", false, "Force overwrite by cleaning up existing tc rules before applying new ones")
}

func start(cmd *cobra.Command, args []string) {
	configFile := args[0]

	// In dry-run mode, we don't need root privileges
	if !dryRun && os.Geteuid() != 0 {
		fmt.Println("Error: this command requires root privileges.")
		os.Exit(1)
	}

	// Load and validate the configuration
	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Printf("Error loading config file: %v\n", err)
		os.Exit(1)
	}

	// Initialize the tc runner
	runner := tc.NewRunner(debug, dryRun)

	// If --force is used, cleanup existing rules first
	if force {
		if errCleanup := runner.Cleanup(cfg); errCleanup != nil {
			fmt.Printf("Error during cleanup: %v\n", errCleanup)
			os.Exit(1)
		}
	}

	// --- Pre-flight checks ---
	// In dry-run mode, we can skip checking if interfaces exist as they might not on the local machine
	if !dryRun {
		// Collect all unique interfaces from rules
		interfaceSet := make(map[string]bool)
		for _, rule := range cfg.Rules {
			interfaceSet[rule.SrcIntf] = true
			interfaceSet[rule.DstIntf] = true
		}

		// Verify all interfaces exist
		for ifaceName := range interfaceSet {
			if _, errIface := net.InterfaceByName(ifaceName); errIface != nil {
				fmt.Printf("Error: interface '%s' not found\n", ifaceName)
				os.Exit(1)
			}
		}
	}

	// Collect unique source interfaces that need clsact qdisc
	srcInterfaceSet := make(map[string]bool)
	for _, rule := range cfg.Rules {
		srcInterfaceSet[rule.SrcIntf] = true
	}

	// Apply the configuration
	// Step 1: Add clsact qdisc to all source interfaces
	for srcIntf := range srcInterfaceSet {
		if errQdisc := runner.EnsureClsactQdisc(srcIntf); errQdisc != nil {
			fmt.Printf("Error: failed to add clsact qdisc to %s: %v\n", srcIntf, errQdisc)
			os.Exit(1)
		}
	}

	// Step 2: Apply filters for each rule
	for _, rule := range cfg.Rules {
		// Direction is always ingress for rules-based config
		direction := "ingress"

		// Apply each filter in the rule
		for _, filter := range rule.Filters {
			if errFilter := runner.AddMirrorFilter(rule.SrcIntf, direction, rule.DstIntf, filter, rule.Rewrite); errFilter != nil {
				fmt.Printf("Error: failed to add filter rule: %v\n", errFilter)
				os.Exit(1)
			}
		}
	}

	if !debug && !dryRun {
		fmt.Println("Started")
	}
}
