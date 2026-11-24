package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"tcbroker/pkg/state"
)

var (
	flagStateFile string
	flagDebug     bool
)

var rootCmd = &cobra.Command{
	Use:   "tcbroker",
	Short: "Stateful CLI tool for Linux TC packet mirroring",
	Long: `tcbroker manages Linux Traffic Control (tc) packet mirroring rules declaratively.

It tracks its own TC filters in a state file and dedicated priority range so you can:
  - Apply configs idempotently with automatic rollback on error
  - Inspect status and real-time counters without passing the config file
  - Delete individual rules or stop everything without affecting other TC tools`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagStateFile, "state-file", "",
		fmt.Sprintf("Path to state file (default %q or $%s)", state.DefaultStatePath, state.EnvStateFile))
	rootCmd.PersistentFlags().BoolVar(&flagDebug, "debug", false, "Print all tc commands executed")
}

func resolvedStatePath() string {
	return state.ResolveStatePath(flagStateFile)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
