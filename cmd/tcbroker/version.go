package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version can be overridden at build time via -ldflags "-X main.version=v2.0.0".
var version = "2.0.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print tcbroker version",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("tcbroker %s\n", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
