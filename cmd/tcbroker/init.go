package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"tcbroker/pkg/config"
)

var initForce bool

var initCmd = &cobra.Command{
	Use:   "init [output-file]",
	Short: "Generate a starter tcbroker YAML configuration file",
	Long: `Writes a documented starter configuration file.
Defaults to ./tcbroker.yaml, or pass '-' to print to stdout.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite output file if it already exists")
}

func runInit(cmd *cobra.Command, args []string) error {
	outPath := "tcbroker.yaml"
	if len(args) == 1 {
		outPath = args[0]
	}

	if outPath == "-" {
		fmt.Print(config.SampleConfigTemplate)
		return nil
	}

	if !initForce {
		if _, err := os.Stat(outPath); err == nil {
			return fmt.Errorf("file %q already exists (use --force to overwrite)", outPath)
		}
	}

	if err := os.WriteFile(outPath, []byte(config.SampleConfigTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outPath, err)
	}

	fmt.Printf("Created %s\n", outPath)
	fmt.Printf("Edit the interface names and run: sudo tcbroker apply %s\n", outPath)
	return nil
}
