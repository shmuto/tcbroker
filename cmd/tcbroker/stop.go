package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"tcbroker/pkg/config"
	"tcbroker/pkg/tc"
)

var stopCmd = &cobra.Command{
	Use:   "stop [config-file]",
	Short: "Stops packet mirroring and cleans up tc rules.",
	Long: `Reads the given YAML configuration file and removes all tc rules
(qdiscs and filters) from the specified interfaces. This command requires root privileges.`,
	Args: cobra.ExactArgs(1),
	Run:  stop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
	stopCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug mode to print tc commands")
	stopCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Enable dry-run mode to print tc commands without executing them")
}

func stop(cmd *cobra.Command, args []string) {
	configFile := args[0]

	// In dry-run mode, we don't need root privileges
	if !dryRun && os.Geteuid() != 0 {
		fmt.Println("Error: this command requires root privileges.")
		os.Exit(1)
	}

	// Load the configuration to know which interfaces to clean up
	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Printf("Error loading config file: %v\n", err)
		os.Exit(1)
	}

	// Initialize the tc runner
	runner := tc.NewRunner(debug, dryRun)

	// Cleanup all tc rules for the interfaces in the config
	if err := runner.Cleanup(cfg); err != nil {
		fmt.Printf("Error: cleanup failed: %v\n", err)
		os.Exit(1)
	}

	if !debug && !dryRun {
		fmt.Println("Stopped")
	}
}
